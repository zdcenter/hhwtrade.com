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

	// 2. Start WebSocket Manager
	go e.websocketHub.Start()

	// 3. Start Market Data Subscriber (Redis Pub/Sub -> infra.MarketDataChan)
	infra.StartMarketDataSubscriber(e.rdb, ctx)

	// 4. Start Event Loop to consume Market Data
	go func() {
		log.Println("Engine Event Loop Started")
		for msg := range infra.MarketDataChan {
			// A. Broadcast to WebSocket Users
			e.websocketHub.Broadcast(msg)

			// B. Trigger Strategies
			// Assuming Payload is JSON with "last_price" or numeric string.
			// Ideally, we should parse it once.
			// Adapt this struct to your actual Redis message format.
			var tickData struct {
				LastPrice float64 `json:"last_price"`
			}
			// Best effort parsing
			if err := json.Unmarshal([]byte(msg.Payload), &tickData); err == nil {
				commands := e.stratExec.OnMarketData(msg.Symbol, tickData.LastPrice)
				for _, cmd := range commands {
					if err := e.SendCommand(context.Background(), *cmd); err != nil {
						log.Printf("Failed to send command for strategy: %v", err)
					} else {
						log.Printf("Strategy Triggered: Sent command %s", cmd.Type)
					}
				}
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
		val, err := e.rdb.BRPop(ctx, 0, infra.ResponseQueueKey).Result()
		if err != nil {
			log.Printf("Error popping from response queue: %v", err)
			time.Sleep(1 * time.Second) // Wait a bit before retrying on error
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
		// Payload example: {"status": "filled", "error_msg": ""}
		statusStr, _ := payload["status"].(string)
		errorMsg, _ := payload["error_msg"].(string) // Optional

		updates := map[string]interface{}{}
		if statusStr != "" {
			updates["status"] = statusStr
		}
		if errorMsg != "" {
			updates["error_msg"] = errorMsg
		}

		if len(updates) > 0 {
			// Update by RequestID (which is unique)
			// In production, matching by RequestID might be tricky if CTP doesn't return it perfectly.
			// Ideally using OrderRef or OrderSysID is better.
			if err := db.Model(&model.Order{}).Where("request_id = ?", resp.RequestID).Updates(updates).Error; err == nil {
				// Push update to WebSocket User
				var order model.Order
				if db.Where("request_id = ?", resp.RequestID).First(&order).Error == nil {
					e.websocketHub.PushToUser(order.UserID, resp)
				}
			}
		}

	case "RTN_TRADE":
		// Trade Execution (Deal)
		// Payload example: {"price": 3500, "volume": 1, "trade_id": "...", "direction": "buy", "offset": "open"}

		// 1. Mark order as Filled (or Partial)
		var order model.Order
		if err := db.Where("request_id = ?", resp.RequestID).First(&order).Error; err == nil {
			db.Model(&order).Update("status", model.OrderStatusFilled)

			// 2. Update Position table (Simple logic)
			e.updatePosition(order, payload)

			// 3. Push update to WebSocket User
			e.websocketHub.PushToUser(order.UserID, resp)
		}

		log.Printf("Trade executed for RequestID %s. Position updated.", resp.RequestID)

	case "ERR_ORDER":
		// Immediate Rejection
		errorMsg, _ := payload["error_msg"].(string)
		if err := db.Model(&model.Order{}).Where("request_id = ?", resp.RequestID).Updates(map[string]interface{}{
			"status":    model.OrderStatusRejected,
			"error_msg": errorMsg,
		}).Error; err == nil {
			// Find order to get UserID
			var order model.Order
			if db.Where("request_id = ?", resp.RequestID).First(&order).Error == nil {
				e.websocketHub.PushToUser(order.UserID, resp)
			}
		}

	case "QRY_POS_RSP":
		// This is a response to a position query command
		// Payload: {"positions": []model.Position}
		if positions, ok := payload["positions"].([]interface{}); ok {
			for _, p := range positions {
				// We need a way to unmarshal map[string]interface{} into model.Position
				pBytes, _ := json.Marshal(p)
				var pos model.Position
				if err := json.Unmarshal(pBytes, &pos); err == nil {
					db.Save(&pos) // Upsert position data
				}
			}
			log.Printf("Synchronized %d positions from CTP Core", len(positions))
		}
	}
}

// updatePosition adjusts the local position record based on a trade execution.
func (e *Engine) updatePosition(order model.Order, tradePayload map[string]interface{}) {
	db := e.pg.DB

	// Determine direction for Position model ("long" or "short")
	// Note: CTP typically separates Long and Short positions for the same symbol.
	posDir := "long"
	if order.Direction == model.DirectionSell && order.Offset == model.OffsetOpen {
		posDir = "short"
	} else if order.Direction == model.DirectionBuy && (order.Offset == model.OffsetClose || order.Offset == model.OffsetCloseToday) {
		posDir = "short"
	}

	var pos model.Position
	err := db.Where("user_id = ? AND symbol = ? AND direction = ?", order.UserID, order.Symbol, posDir).First(&pos).Error

	tradeVol := order.Volume // In a real system, use volume from tradePayload if partial fill
	tradePrice, _ := tradePayload["price"].(float64)

	if err != nil {
		// New position
		if order.Offset == model.OffsetOpen {
			pos = model.Position{
				UserID:       order.UserID,
				Symbol:       order.Symbol,
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
func (e *Engine) QueryPositions(userID string) error {
	cmd := infra.TradeCommand{
		Type: "QUERY_POSITIONS",
		Payload: map[string]string{
			"user_id": userID,
		},
		RequestID: "query-pos-" + time.Now().Format("20060102150405"),
	}
	return e.SendCommand(context.Background(), cmd)
}

// SendCommand wraps infra.SendTradeCommand using the engine's Redis client.
func (e *Engine) SendCommand(ctx context.Context, cmd infra.TradeCommand) error {
	return infra.SendTradeCommand(ctx, e.rdb, cmd)
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
