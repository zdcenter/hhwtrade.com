package main

import (
	"context"
	"log"

	"hhwtrade.com/internal/api"
	"hhwtrade.com/internal/config"
	"hhwtrade.com/internal/engine"
	"hhwtrade.com/internal/infra"
)

func main() {
	// 1. 加载配置
	cfg := config.LoadConfig()

	// 2. 初始化基础设施
	// Postgres
	pg, err := infra.NewPostgresClient(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Redis
	rdb := infra.NewRedisClient(cfg.Redis)
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// 3. 初始化 WebSocket 管理器
	wsHub := infra.NewWsManager()

	// 4. 初始化引擎
	eng := engine.NewEngine(cfg, pg, rdb, wsHub)

	// 启动引擎（启动后台进程，如 WebSocket Hub 和 Redis 订阅器）
	ctx := context.Background()
	eng.Start(ctx)

	// 5. 设置 Fiber 服务器
	app := api.NewServer(cfg, eng)

	// 6. 启动服务器
	log.Printf("Server starting on port %s", cfg.Server.Port)
	if err := app.Listen(cfg.Server.Port); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
