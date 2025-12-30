package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"hhwtrade.com/internal/config"
	"hhwtrade.com/internal/engine"
)

func NewServer(cfg *config.Config, eng *engine.Engine) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: cfg.Server.AppName,
	})

	app.Use(logger.New())
	app.Use(cors.New())

	// 初始化并注册路由
	router := NewRouter(app, cfg, eng)
	router.RegisterRoutes()

	return app
}
