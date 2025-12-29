package engine

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"hhwtrade.com/internal/config"
	"hhwtrade.com/internal/ctp"
	"hhwtrade.com/internal/infra"
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

	// CTP Components
	ctpClient  *ctp.Client
	ctpHandler *ctp.Handler
}

// NewEngine creates a new Engine instance.
func NewEngine(cfg *config.Config, pg *infra.PostgresClient, rdb *redis.Client, wsHub *infra.WsManager) *Engine {
	// Initialize Strategy Executor
	exec := strategies.NewExecutor(pg.DB)

	// Initialize CTP Components
	ctpClient := ctp.NewClient(rdb)
	ctpHandler := ctp.NewHandler(pg.DB, wsHub)

	return &Engine{
		cfg:          cfg,
		pg:           pg,
		rdb:          rdb,
		websocketHub: wsHub,
		subs:         NewSubscriptionState(),
		stratExec:    exec,
		ctpClient:    ctpClient,
		ctpHandler:   ctpHandler,
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
						_ = e.ctpClient.InsertOrder(context.Background(), cmd)
					}
				}
			} else {
				// B. It's a Query Response from Pub/Sub (Symbol is empty)
				// We reuse the TradeResponse handler logic for consistency, assuming the payload structure matches
				e.handleRawResponse(string(msg.Payload))
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
		val, err := e.rdb.BRPop(ctx, 0, ctp.PushCtpTradeReportList).Result()
		if err != nil {
			log.Printf("Error popping from response queue: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// val[1] is the JSON payload string
		e.handleRawResponse(val[1])
	}
}

// handleRawResponse unmarshals the JSON and delegates to ctpHandler
func (e *Engine) handleRawResponse(jsonStr string) {
	var resp ctp.TradeResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		log.Printf("Failed to unmarshal trade response: %v", err)
		return
	}
	e.ctpHandler.ProcessResponse(resp)
}

// QueryPositions sends a command to CTP Core to fetch all positions for a user.
func (e *Engine) QueryPositions(userID string, instrumentID string) error {
	return e.ctpClient.QueryPositions(context.Background(), userID, instrumentID)
}

// QueryAccount sends a command to CTP Core to fetch trading account info.
func (e *Engine) QueryAccount(userID string) error {
	return e.ctpClient.QueryAccount(context.Background(), userID)
}

// SyncInstruments triggers CTP Core to fetch all available instruments.
func (e *Engine) SyncInstruments() error {
	log.Println("Engine: Triggering Instrument Sync from CTP Core")
	return e.ctpClient.SyncInstruments(context.Background())
}

// SubscribeSymbol adds a symbol to the engine's tracking and sends a subscribe command to CTP if it's new.
func (e *Engine) SubscribeSymbol(instrumentID string) error {
	isFirst := e.subs.AddSubscription(instrumentID)

	if isFirst {
		log.Printf("Engine: New subscription for %s, sending command to CTP", instrumentID)
		return e.ctpClient.Subscribe(context.Background(), instrumentID)
	}
	return nil
}

// UnsubscribeSymbol removes a symbol reference and sends an unsubscribe command if it's the last one.
func (e *Engine) UnsubscribeSymbol(instrumentID string) error {
	if e.subs.RemoveSubscription(instrumentID) {
		log.Printf("Engine: No more subscribers for %s, sending unsubscribe to CTP", instrumentID)
		return e.ctpClient.Unsubscribe(context.Background(), instrumentID)
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

func (e *Engine) GetCtpClient() *ctp.Client {
	return e.ctpClient
}



