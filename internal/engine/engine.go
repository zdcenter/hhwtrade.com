package engine

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"hhwtrade.com/internal/config"
	"hhwtrade.com/internal/constants"
	"hhwtrade.com/internal/ctp"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/infra"
	"hhwtrade.com/internal/service"
)

// Engine 是一个轻量级协调器，负责：
// 1. 启动后台进程（行情监听、交易回报监听）
// 2. 将行情数据分发给 WebSocket 和策略服务
// 3. 协调各服务之间的交互
type Engine struct {
	cfg *config.Config

	// 基础设施
	rdb          *redis.Client
	websocketHub *infra.WsManager
	ctpHandler   *ctp.Handler

	// 业务服务 (依赖接口)
	marketService   *service.MarketServiceImpl
	strategyService *service.StrategyServiceImpl

	// 上下文控制
	ctx    context.Context
	cancel context.CancelFunc
}

// NewEngine 创建引擎
func NewEngine(
	cfg *config.Config,
	rdb *redis.Client,
	websocketHub *infra.WsManager,
	ctpHandler *ctp.Handler,
	marketService *service.MarketServiceImpl,
	strategyService *service.StrategyServiceImpl,
) *Engine {
	ctx, cancel := context.WithCancel(context.Background())

	return &Engine{
		cfg:             cfg,
		rdb:             rdb,
		websocketHub:    websocketHub,
		ctpHandler:      ctpHandler,
		marketService:   marketService,
		strategyService: strategyService,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start 启动引擎后台进程
func (e *Engine) Start() {
	log.Println("Engine: Starting...")

	// 1. 加载活跃策略
	e.strategyService.LoadActiveStrategies()

	// 2. 为活跃策略订阅行情
	for _, symbol := range e.strategyService.GetActiveSymbols() {
		log.Printf("Engine: Subscribing to %s for active strategies", symbol)
		e.marketService.AddExistingSubscription(symbol)
		if err := e.marketService.Subscribe(e.ctx, symbol); err != nil {
			log.Printf("Engine: Failed to subscribe to %s: %v", symbol, err)
		}
	}

	// 3. 启动 WebSocket 管理器
	go e.websocketHub.Start()

	// 4. 启动行情数据订阅器
	infra.StartMarketDataSubscriber(e.rdb, e.ctx)
	infra.StartQueryReplySubscriber(e.rdb, e.ctx)
	infra.StartStatusSubscriber(e.rdb, e.marketService, e.ctx)

	// 5. (已移除) 启动行情分发循环 (由 Dispatcher 接管)
	// go e.runMarketDataLoop()

	// 6. 启动交易回报监听
	go e.runTradeResponseLoop()

	log.Println("Engine: Started successfully")
}

// OnMarketData 接收并处理行情数据 (由 Dispatcher 调用)
func (e *Engine) OnMarketData(msg infra.MarketMessage) {
	if msg.Symbol != "" {
		// 1. (原逻辑中此处为广播 websocket，现已移除，专注策略)

		// 2. 解析价格，触发策略
		var tickData struct {
			LastPrice float64 `json:"LastPrice"`
		}
		if err := json.Unmarshal([]byte(msg.Payload), &tickData); err == nil {
			e.strategyService.OnMarketData(e.ctx, msg.Symbol, tickData.LastPrice)
		}
	} else {
		// 查询响应
		e.handleQueryResponse(msg.Payload)
	}
}

// handleQueryResponse 处理查询响应
func (e *Engine) handleQueryResponse(payload json.RawMessage) {
	var resp ctp.TradeResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		log.Printf("Engine: Failed to unmarshal query response: %v", err)
		return
	}
	e.ctpHandler.ProcessResponse(resp)
}

// runTradeResponseLoop 交易回报监听循环
func (e *Engine) runTradeResponseLoop() {
	log.Println("Engine: Trade response loop started")

	for {
		select {
		case <-e.ctx.Done():
			log.Println("Engine: Trade response loop stopped")
			return
		default:
			// BRPOP 阻塞等待，超时 1 秒
			val, err := e.rdb.BRPop(e.ctx, 1*time.Second, constants.RedisQueueCTPResponse).Result()
			if err != nil {
				if err == redis.Nil {
					continue // 超时，继续循环
				}
				if e.ctx.Err() != nil {
					return // 上下文取消
				}
				log.Printf("Engine: Error reading trade response: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}

			// val[1] 是 JSON 数据
			var resp ctp.TradeResponse
			if err := json.Unmarshal([]byte(val[1]), &resp); err != nil {
				log.Printf("Engine: Failed to unmarshal trade response: %v", err)
				continue
			}

			e.ctpHandler.ProcessResponse(resp)
		}
	}
}

// Stop 停止引擎
func (e *Engine) Stop() {
	log.Println("Engine: Stopping...")
	e.cancel()
}

// GetNotifier 返回 WebSocket 通知器 (实现 domain.Notifier 接口)
func (e *Engine) GetNotifier() domain.Notifier {
	return e.websocketHub
}

// GetWebSocketHub 返回 WebSocket 管理器
func (e *Engine) GetWebSocketHub() *infra.WsManager {
	return e.websocketHub
}
