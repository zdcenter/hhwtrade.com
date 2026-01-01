package infra

import (
	"log"
)

// MarketDataDispatcher is responsible for distributing market data from Redis to various consumers.
type MarketDataDispatcher struct {
	wsManager *WsManager
	engine    StrategyHandler
}

// StrategyHandler defines the interface for components that need to process market data for trading strategies.
type StrategyHandler interface {
	OnMarketData(msg MarketMessage)
}

// NewMarketDataDispatcher creates a new dispatcher instance.
func NewMarketDataDispatcher(wsManager *WsManager, engine StrategyHandler) *MarketDataDispatcher {
	return &MarketDataDispatcher{
		wsManager: wsManager,
		engine:    engine,
	}
}

// Start begins listening to the MarketDataChan and dispatching messages.
// It should be run in a separate goroutine.
func (d *MarketDataDispatcher) Start() {
	log.Println("MarketDataDispatcher: Started listening for market data...")
	for msg := range MarketDataChan {
		// 1. Dispatch to WebSocket Clients (UI)
		// We use a non-blocking approach implementation inside WsManager usually,
		// but here we just call Broadcast which is thread-safe.
		d.wsManager.Broadcast(msg)

		// 2. Dispatch to Engine (Strategy)
		// This is done sequentially here to ensure order, but could be parallelized if needed.
		// Since Engine logic can be complex, catching panics here is a good idea to prevent the dispatcher from crashing.
		d.safeCallEngine(msg)
	}
	log.Println("MarketDataDispatcher: MarketDataChan closed, stopping.")
}

func (d *MarketDataDispatcher) safeCallEngine(msg MarketMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("MarketDataDispatcher: Panic in Engine.OnMarketData: %v", r)
		}
	}()
	d.engine.OnMarketData(msg)
}
