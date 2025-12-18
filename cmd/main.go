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
	// 1. Load Configuration
	cfg := config.LoadConfig()

	// 2. Initialize Infrastructure
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

	// 3. Initialize Engine
	// We use the global WsManager for now as it's initialized in infra package
	eng := engine.NewEngine(cfg, pg, rdb, infra.GlobalWsManager)

	// Start Engine (starts background processes like WebSocket Hub and Redis Subscriber)
	ctx := context.Background()
	eng.Start(ctx)

	// 4. Setup Fiber Server
	app := api.NewServer(cfg, eng)

	// 5. Start Server
	log.Printf("Server starting on port %s", cfg.Server.Port)
	if err := app.Listen(cfg.Server.Port); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
