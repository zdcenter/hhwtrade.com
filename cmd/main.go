package main

import (
	"context"
	"log"

	"hhwtrade.com/internal/api"
	"hhwtrade.com/internal/config"
	"hhwtrade.com/internal/ctp"
	"hhwtrade.com/internal/engine"
	"hhwtrade.com/internal/infra"
	"hhwtrade.com/internal/service"
	"hhwtrade.com/internal/strategies"
)

func main() {
	// ============================================
	// 1. 加载配置
	// ============================================
	cfg := config.LoadConfig()

	// ============================================
	// 2. 初始化基础设施层
	// ============================================

	// 2.1 Postgres
	pg, err := infra.NewPostgresClient(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// 2.2 Redis
	rdb := infra.NewRedisClient(cfg.Redis)
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// 2.3 WebSocket 管理器
	wsHub := infra.NewWsManager()

	// ============================================
	// 3. 初始化 CTP 层
	// ============================================

	// 3.1 CTP Client (发送指令)
	ctpClient := ctp.NewClient(rdb)

	// 3.2 CTP Handler (处理回报)
	ctpHandler := ctp.NewCTPHandler(pg.DB, wsHub)

	// ============================================
	// 4. 初始化服务层
	// ============================================

	// 4.1 行情服务
	marketService := service.NewMarketService(ctpClient, wsHub)

	// 4.2 交易服务
	tradingService := service.NewTradingService(pg.DB, ctpClient, wsHub)

	// 4.3 策略执行器
	strategyExecutor := strategies.NewExecutor(pg.DB)

	// 4.4 策略服务
	strategyService := service.NewStrategyService(pg.DB, strategyExecutor, tradingService)

	// 4.5 订阅服务
	subscriptionService := service.NewSubscriptionService(pg.DB, marketService, wsHub)
	if err := subscriptionService.RestoreSubscriptions(context.Background()); err != nil {
		log.Printf("Warning: Failed to restore subscriptions: %v", err)
	}

	// ============================================
	// 5. 初始化引擎 (协调器)
	// ============================================
	eng := engine.NewEngine(
		cfg,
		rdb,
		wsHub,
		ctpHandler,
		marketService,
		strategyService,
	)

	// 启动引擎后台进程
	eng.Start()

	// ============================================
	// 5.1 启动行情分发器 (新架构)
	// ============================================
	// 负责将 Redis 行情分发给 WebSocket (UI) 和 Engine (策略)
	dispatcher := infra.NewMarketDataDispatcher(wsHub, eng)
	go dispatcher.Start()

	// ============================================
	// 6. 初始化 HTTP 服务器
	// ============================================
	app := api.NewServer(cfg)

	// 配置路由 (依赖注入)
	api.SetupRoutes(app, api.RouterDeps{
		App:             app,
		Cfg:             cfg,
		DB:              pg.DB,
		WsHub:           wsHub,
		SubscriptionSvc: subscriptionService,
		TradingSvc:      tradingService,
		StrategySvc:     strategyService,
		MarketSvc:       marketService,
	})

	// ============================================
	// 7. 启动服务器
	// ============================================
	log.Printf("Server starting on port %s", cfg.Server.Port)
	if err := app.Listen(cfg.Server.Port); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
