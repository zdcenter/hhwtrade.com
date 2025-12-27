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
		statusStr, _ := payload["status"].(string)
		orderSysID, _ := payload["order_sys_id"].(string)
		errorMsg, _ := payload["error_msg"].(string)
		
		var order model.Order
		if err := db.Where("order_ref = ?", resp.RequestID).First(&order).Error; err == nil {
			// Record Log
			db.Create(&model.OrderStatusLog{
				OrderID:   order.ID,
				OldStatus: string(order.Status),
				NewStatus: statusStr,
				Message:   errorMsg,
				CreatedAt: time.Now(),
			})

			updates := map[string]interface{}{}
			if statusStr != "" {
				updates["status"] = statusStr
			}
			if orderSysID != "" {
				updates["order_sys_id"] = orderSysID
			}
			if errorMsg != "" {
				updates["error_msg"] = errorMsg
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
			tradeVol, _ := payload["volume"].(float64) 
			price, _ := payload["price"].(float64)
			tradeID, _ := payload["trade_id"].(string)

			// 1. Insert Trade Record
				db.Create(&model.TradeRecord{
				OrderID:      order.ID,
				TicketNo:     order.OrderRef,
				TradeID:      tradeID,
				InstrumentID: order.InstrumentID,
				Direction:    string(order.Direction),
				Offset:       string(order.Offset),
				Price:     price,
				Volume:    int(tradeVol),
				TradeTime: time.Now().Format("15:04:05"), 
			})

			// 2. Partial Fill Logic
			newFilledVol := order.FilledVolume + int(tradeVol)
			updates := map[string]interface{}{
				"filled_volume": newFilledVol,
			}

			if newFilledVol >= order.Volume {
				updates["status"] = model.OrderStatusAllTraded
			} else {
				updates["status"] = model.OrderStatusPartTradedQueueing
			}

			db.Model(&order).Updates(updates)

			// 3. Update Position
			e.updatePosition(order, payload)

			// 4. Notify user
			e.websocketHub.PushToUser(order.UserID, resp)
		}
		log.Printf("Trade for %s: Volume %v", resp.RequestID, payload["volume"])

	case "ERR_ORDER":
		// Immediate Rejection
		errorMsg, _ := payload["error_msg"].(string)
		
		var order model.Order
		if db.Where("order_ref = ?", resp.RequestID).First(&order).Error == nil {
			// Log Rejection
			db.Create(&model.OrderStatusLog{
				OrderID:   order.ID,
				OldStatus: string(order.Status),
				NewStatus: string(model.OrderStatusNoTradeNotQueueing), // Rejected/Failed
				Message:   errorMsg,
				CreatedAt: time.Now(),
			})

			db.Model(&order).Updates(map[string]interface{}{
				"status":    model.OrderStatusNoTradeNotQueueing,
				"error_msg": errorMsg,
			})
			e.websocketHub.PushToUser(order.UserID, resp)
		}

	case "QRY_POS_RSP":
		// This is a response to a position query command
		// Payload: {"positions": []model.Position}
		if positions, ok := payload["positions"].([]interface{}); ok {
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
		// Payload: {"instruments": []model.FuturesContract}
		log.Printf("Received QRY_INSTRUMENT_RSP: %v", payload)	
		if instruments, ok := payload["instruments"].([]interface{}); ok {
			for _, inst := range instruments {
				instBytes, _ := json.Marshal(inst)
				var instrument model.FuturesContract
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

	// Determine direction for Position model ("long" or "short")
	// Note: CTP typically separates Long and Short positions for the same InstrumentID.
	posDir := "long"
	if order.Direction == model.DirectionSell && order.Offset == model.OffsetOpen {
		posDir = "short"
	} else if order.Direction == model.DirectionBuy && (order.Offset == model.OffsetClose || order.Offset == model.OffsetCloseToday) {
		posDir = "short"
	}

	var pos model.Position
	err := db.Where("user_id = ? AND instrument_id = ? AND direction = ?", order.UserID, order.InstrumentID, posDir).First(&pos).Error

	tradeVol := order.Volume // In a real system, use volume from tradePayload if partial fill
	tradePrice, _ := tradePayload["price"].(float64)

	if err != nil {
		// New position
		if order.Offset == model.OffsetOpen {
			pos = model.Position{
				UserID:       order.UserID,
				InstrumentID: order.InstrumentID,
				Direction:    posDir,
				TotalVolume:  tradeVol,
				TodayVolume:  tradeVol,
				AveragePrice: tradePrice,
				UpdatedAt:    time.Now(),
			}
			db.Create(&pos)
		}
	} else {
		// Existing position
		if order.Offset == model.OffsetOpen {
			// Increase position and recalculate average price
			newTotal := pos.TotalVolume + tradeVol
			pos.AveragePrice = (pos.AveragePrice*float64(pos.TotalVolume) + tradePrice*float64(tradeVol)) / float64(newTotal)
			pos.TotalVolume = newTotal
			pos.TodayVolume += tradeVol
		} else {
			// Decrease position
			pos.TotalVolume -= tradeVol
			if pos.TotalVolume < 0 {
				pos.TotalVolume = 0 // Safety check
			}
			// Note: TodayVolume logic depends on whether it's SHFE (CloseToday) or others
			if order.Offset == model.OffsetCloseToday {
				pos.TodayVolume -= tradeVol
			}
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
			"user_id": userID,
			"instrument_id":  instrumentID,
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
			"user_id": userID,
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
				"instrument_id": instrumentID,
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
				"instrument_id": instrumentID,
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
