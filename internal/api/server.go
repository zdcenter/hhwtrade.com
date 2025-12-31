package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"hhwtrade.com/internal/config"
)

// NewServer 创建 Fiber 服务器
func NewServer(cfg *config.Config) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: cfg.Server.AppName,
	})

	app.Use(logger.New())
	app.Use(cors.New())

	return app
}

// SetupRoutes 配置路由 (在所有依赖准备好之后调用)
func SetupRoutes(app *fiber.App, deps RouterDeps) {
	router := NewRouter(deps)
	router.RegisterRoutes()
}
