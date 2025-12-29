package engine

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"hhwtrade.com/internal/config"
	"hhwtrade.com/internal/infra"
	"hhwtrade.com/internal/model"
	"hhwtrade.com/internal/strategies"
)

// Engine is the central monolithic service that coordinates database, redis, and websocket.
type Engine struct {
	// cfg Global configuration
	cfg *config.Config

	// postgres client
	pg *infra.PostgresClient
	// rdb Redis client (used for List queues and Pub/Sub)
	rdb *redis.Client

	// hub WebSocket Hub/Manager for broadcasts and user push
	websocketHub *infra.WsManager

	// subs Subscription state (tracks user symbol subscriptions)
	subs *SubscriptionState

	// stratExec Strategy Executor
	stratExec *strategies.Executor
}

// NewEngine creates a new Engine instance.
func NewEngine(cfg *config.Config, pg *infra.PostgresClient, rdb *redis.Client, wsHub *infra.WsManager) *Engine {
	// Initialize Strategy Executor
	exec := strategies.NewExecutor(pg.DB)

	return &Engine{
		cfg:          cfg,
		pg:           pg,
		rdb:          rdb,
		websocketHub: wsHub,
		subs:         NewSubscriptionState(),
		stratExec:    exec,
	}
}

// Start initializes background processes like the market data subscriber.
func (e *Engine) Start(ctx context.Context) {
	log.Println("Starting Engine...")

	// 1. Load Strategies into Memory
	e.stratExec.LoadActiveStrategies()

	// 1.1 Trigger CTP Subscriptions for Active Strategies
	// This ensures we get data even if no UI user is watching
	for _, instID := range e.stratExec.GetSymbols() {
		log.Printf("Engine: Subscribing to %s for active strategies", instID)
		e.SubscribeSymbol(instID)
	}

	// 2. Start WebSocket Manager
	go e.websocketHub.Start()

	// 3. Start Market Data & Query Subscriber (Redis Pub/Sub)
	infra.StartMarketDataSubscriber(e.rdb, ctx)
	infra.StartQueryReplySubscriber(e.rdb, ctx)

	// 4. Start Event Loop
	go func() {
		log.Println("Engine Event Loop Started")
		for msg := range infra.MarketDataChan {
			// A. If it's a market tick (InstrumentID is not empty)
			if msg.Symbol != "" {
				e.websocketHub.Broadcast(msg)

				var tickData struct {
					LastPrice float64 `json:"LastPrice"`
				}
				if err := json.Unmarshal([]byte(msg.Payload), &tickData); err == nil {
					// NOTE: we keep msg.Symbol for internal websocket protocol, 
					// but strategy might want InstrumentID
					commands := e.stratExec.OnMarketData(msg.Symbol, tickData.LastPrice)
					for _, cmd := range commands {
						_ = e.SendCommand(context.Background(), *cmd)
					}
				}
			} else {
				// B. It's a Query Response from Pub/Sub (Symbol is empty)
				e.handleTradeResponse(string(msg.Payload))
			}
		}
	}()

	// 5. Start Trade Response Listener (CTP -> Go)
	go e.listenTradeResponses()

	log.Println("Engine started.")
}

// listenTradeResponses constantly consumers messages from the Redis response queue.
func (e *Engine) listenTradeResponses() {
	log.Println("Starting Trade Response Listener...")
	ctx := context.Background()
	for {
		// BRPOP blocks until data is available. 0 means block indefinitely.
		// Returns [key, value]
		val, err := e.rdb.BRPop(ctx, 0, infra.PushCtpTradeReportList).Result()
		if err != nil {
			log.Printf("Error popping from response queue: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// val[1] is the JSON payload string
		e.handleTradeResponse(val[1])
	}
}

// handleTradeResponse parses and processes the trade response.
func (e *Engine) handleTradeResponse(jsonStr string) {
	var resp infra.TradeResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		log.Printf("Failed to unmarshal trade response: %v", err)
		return
	}

	log.Printf("Received Trade Response: Type=%s, ReqID=%s", resp.Type, resp.RequestID)

	// Since decoding the dynamic Payload map can be tedious,
	// we assume CTP sends us fields like "status_msg" or "status" in the payload map for now.
	// You might want to define concrete structs for OrderPayload and TradePayload later.

	payload, ok := resp.Payload.(map[string]interface{})
	if !ok {
		log.Printf("Invalid payload format")
		return
	}

	db := e.pg.DB

	switch resp.Type {
	case "RTN_ORDER":
		// Order Status Update
		// Payload: {"status": "1", "order_sys_id": "12345", "error_msg": ""}
		statusStr, _ := payload["OrderStatus"].(string)
		orderSysID, _ := payload["OrderSysID"].(string)
		errorMsg, _ := payload["StatusMsg"].(string)
		
		var order model.Order
		if err := db.Where("order_ref = ?", resp.RequestID).First(&order).Error; err == nil {
			// Record Log
			db.Create(&model.OrderLog{
				OrderID:   order.ID,
				OldStatus: string(order.OrderStatus),
				NewStatus: statusStr,
				Message:   errorMsg,
				CreatedAt: time.Now(),
			})

			updates := map[string]interface{}{}
			if statusStr != "" {
				updates["OrderStatus"] = statusStr
			}
			if orderSysID != "" {
				updates["OrderSysID"] = orderSysID
			}
			if errorMsg != "" {
				updates["StatusMsg"] = errorMsg
			}

			if len(updates) > 0 {
				db.Model(&order).Updates(updates)
				// Notify User
				e.websocketHub.PushToUser(order.UserID, resp)
			}
		}

	case "RTN_TRADE":
		// Trade Execution (Deal)
		// Payload: {"price": 3500, "volume": 1, "trade_id": "T1", "direction": "0", "offset": "0"}

		var order model.Order
		if err := db.Where("order_ref = ?", resp.RequestID).First(&order).Error; err == nil {
			tradeVol, _ := payload["Volume"].(float64) 
			price, _ := payload["Price"].(float64)
			tradeID, _ := payload["TradeID"].(string)

			// 1. Insert Trade Record
				db.Create(&model.Trade{
				OrderID:      order.ID,
				OrderRef:     order.OrderRef,
				OrderSysID:   order.OrderSysID,
				TradeID:      tradeID,
				InstrumentID: order.InstrumentID,
				Direction:    string(order.Direction),
				OffsetFlag:   string(order.CombOffsetFlag),
				Price:        price,
				Volume:       int(tradeVol),
				TradeTime:    time.Now().Format("15:04:05"), 
			})

			// 2. Partial Fill Logic
			newFilledVol := order.VolumeTraded + int(tradeVol)
			updates := map[string]interface{}{
				"VolumeTraded": newFilledVol,
			}

			if newFilledVol >= order.VolumeTotalOriginal {
				updates["OrderStatus"] = model.OrderStatusAllTraded
			} else {
				updates["OrderStatus"] = model.OrderStatusPartTradedQueueing
			}

			db.Model(&order).Updates(updates)

			// 3. Update Position
			e.updatePosition(order, payload)

			// 4. Notify user
			e.websocketHub.PushToUser(order.UserID, resp)
		}
		log.Printf("Trade for %s: Volume %v", resp.RequestID, payload["Volume"])

	case "ERR_ORDER":
		// Immediate Rejection
		errorMsg, _ := payload["ErrorMsg"].(string)
		
		var order model.Order
		if db.Where("order_ref = ?", resp.RequestID).First(&order).Error == nil {
			// Log Rejection
			db.Create(&model.OrderLog{
				OrderID:   order.ID,
				OldStatus: string(order.OrderStatus),
				NewStatus: string(model.OrderStatusNoTradeNotQueueing), // Rejected/Failed
				Message:   errorMsg,
				CreatedAt: time.Now(),
			})

			db.Model(&order).Updates(map[string]interface{}{
				"OrderStatus": model.OrderStatusNoTradeNotQueueing,
				"StatusMsg":   errorMsg,
			})
			e.websocketHub.PushToUser(order.UserID, resp)
		}

	case "QRY_POS_RSP":
		// This is a response to a position query command
		// Payload: {"positions": []model.Position}
		if positions, ok := payload["Positions"].([]interface{}); ok {
			for _, p := range positions {
				pBytes, _ := json.Marshal(p)
				var pos model.Position
				if err := json.Unmarshal(pBytes, &pos); err == nil {
					db.Save(&pos) // Upsert position data
				}
			}
			log.Printf("Synchronized %d positions from CTP Core", len(positions))
		}

	case "QRY_INSTRUMENT_RSP":
		// This is a response to an instrument query command
		// Payload: {"instruments": []model.Future}
		log.Printf("Received QRY_INSTRUMENT_RSP: %v", payload)	
		if instruments, ok := payload["Instruments"].([]interface{}); ok {
			for _, inst := range instruments {
				instBytes, _ := json.Marshal(inst)
				var instrument model.Future
				if err := json.Unmarshal(instBytes, &instrument); err == nil {
					// Upsert instrument data
					db.Save(&instrument)
				}
			}
			log.Printf("Synchronized %d instruments from CTP Core", len(instruments))
		}
	}
}

// updatePosition adjusts the local position record based on a trade execution.
func (e *Engine) updatePosition(order model.Order, tradePayload map[string]interface{}) {
	db := e.pg.DB

	// Determine PosiDirection: '2' Long, '3' Short
	posiDir := "2" // Default to Long
	if order.Direction == model.DirectionBuy {
		if order.CombOffsetFlag != model.OffsetOpen {
			posiDir = "3" // Buy Close -> belongs to Short side
		}
	} else {
		if order.CombOffsetFlag == model.OffsetOpen {
			posiDir = "3" // Sell Open -> belongs to Short side
		}
	}

	var pos model.Position
	err := db.Where("user_id = ? AND instrument_id = ? AND posi_direction = ?", order.UserID, order.InstrumentID, posiDir).First(&pos).Error

	tradeVol, _ := tradePayload["Volume"].(float64) // Get actual traded volume from CTP payload
	tradePrice, _ := tradePayload["Price"].(float64)

	if err != nil {
		// New position
		if order.CombOffsetFlag == model.OffsetOpen {
			pos = model.Position{
				UserID:        order.UserID,
				InstrumentID:  order.InstrumentID,
				PosiDirection: posiDir,
				Position:      int(tradeVol),
				TodayPosition: int(tradeVol),
				AveragePrice:  tradePrice,
				PositionCost:  tradePrice * tradeVol, // Initial cost
				UpdatedAt:    time.Now(),
			}
			db.Create(&pos)
		}
	} else {
		// Existing position
		if order.CombOffsetFlag == model.OffsetOpen {
			// Increase position and recalculate average price
			newTotal := pos.Position + int(tradeVol)
			// Recalculate AveragePrice based on cost
			pos.PositionCost += tradePrice * tradeVol
			pos.AveragePrice = pos.PositionCost / float64(newTotal)
			pos.Position = newTotal
			pos.TodayPosition += int(tradeVol)
		} else {
			// Decrease position
			pos.Position -= int(tradeVol)
			if pos.Position < 0 {
				pos.Position = 0
			}
			// SHFE CloseToday logic
			if order.CombOffsetFlag == model.OffsetCloseToday {
				pos.TodayPosition -= int(tradeVol)
			} else {
				pos.YdPosition -= int(tradeVol)
			}
			if pos.TodayPosition < 0 { pos.TodayPosition = 0 }
			if pos.YdPosition < 0 { pos.YdPosition = 0 }
		}
		pos.UpdatedAt = time.Now()
		db.Save(&pos)
	}
}

// QueryPositions sends a command to CTP Core to fetch all positions for a user.
func (e *Engine) QueryPositions(userID string, instrumentID string) error {
	cmd := infra.Command{
		Type: "QUERY_POSITIONS",
		Payload: map[string]interface{}{
			"InvestorID":   userID,
			"InstrumentID": instrumentID,
		},
		RequestID: "query-pos-" + time.Now().Format("20060102150405"),
	}
	return e.SendCommand(context.Background(), cmd)
}

// QueryAccount sends a command to CTP Core to fetch trading account info.
func (e *Engine) QueryAccount(userID string) error {
	cmd := infra.Command{
		Type: "QUERY_ACCOUNT",
		Payload: map[string]interface{}{
			"InvestorID": userID,
		},
		RequestID: "query-acc-" + time.Now().Format("20060102150405"),
	}
	return e.SendCommand(context.Background(), cmd)
}

// SyncInstruments triggers CTP Core to fetch all available instruments.
func (e *Engine) SyncInstruments() error {
	cmd := infra.Command{
		Type:      "QUERY_INSTRUMENTS",
		Payload:   map[string]interface{}{}, // Empty payload for all instruments
		RequestID: "sync-inst-" + time.Now().Format("20060102150405"),
	}
	log.Println("Engine: Triggering Instrument Sync from CTP Core")
	return e.SendCommand(context.Background(), cmd)
}

// SendCommand wraps infra.SendCommand using the engine's Redis client.
func (e *Engine) SendCommand(ctx context.Context, cmd infra.Command) error {
	return infra.SendCommand(ctx, e.rdb, cmd)
}

// SubscribeSymbol adds a symbol to the engine's tracking and sends a subscribe command to CTP if it's new.
func (e *Engine) SubscribeSymbol(instrumentID string) error {
	isFirst := e.subs.AddSubscription(instrumentID)

	if isFirst {
		log.Printf("Engine: New subscription for %s, sending command to CTP", instrumentID)
		cmd := infra.Command{
			Type: "SUBSCRIBE",
			Payload: map[string]interface{}{
				"InstrumentID": instrumentID,
			},
			RequestID: "sub-" + instrumentID + "-" + time.Now().Format("20060102150405"),
		}
		return e.SendCommand(context.Background(), cmd)
	}
	return nil
}

// UnsubscribeSymbol removes a symbol reference and sends an unsubscribe command if it's the last one.
func (e *Engine) UnsubscribeSymbol(instrumentID string) error {
	if e.subs.RemoveSubscription(instrumentID) {
		log.Printf("Engine: No more subscribers for %s, sending unsubscribe to CTP", instrumentID)
		cmd := infra.Command{
			Type: "UNSUBSCRIBE",
			Payload: map[string]interface{}{
				"InstrumentID": instrumentID,
			},
			RequestID: "unsub-" + instrumentID + "-" + time.Now().Format("20060102150405"),
		}
		return e.SendCommand(context.Background(), cmd)
	}
	return nil
}

// GetSubscriptionState returns the subscription state manager.
func (e *Engine) GetSubscriptionState() *SubscriptionState {
	return e.subs
}

// GetWsManager returns the WebSocket manager.
func (e *Engine) GetWsManager() *infra.WsManager {
	return e.websocketHub
}

// GetRedisClient returns the Redis client.
func (e *Engine) GetRedisClient() *redis.Client {
	return e.rdb
}

func (e *Engine) GetConfig() *config.Config {
	return e.cfg
}

func (e *Engine) GetPostgresClient() *infra.PostgresClient {
	return e.pg
}
