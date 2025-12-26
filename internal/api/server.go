package api

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"hhwtrade.com/internal/api/middleware"
	"hhwtrade.com/internal/auth"
	"hhwtrade.com/internal/config"
	"hhwtrade.com/internal/engine"
)

func NewServer(cfg *config.Config, eng *engine.Engine) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: cfg.Server.AppName,
	})

	app.Use(logger.New())
	app.Use(cors.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":  "ok",
			"message": "Service is healthy",
		})
	})

	// Initialize WebSocket
	InitWebsocket(app, eng)

	// Initialize Subscription Handler
	subHandler := NewSubscriptionHandler(eng)
	strategyHandler := NewStrategyHandler(eng)

	// Initialize Casbin Enforcer
	enforcer, err := auth.InitCasbin(eng.GetPostgresClient().DB)
	if err != nil {
		log.Fatalf("Failed to initialize Casbin: %v", err)
	}

	// Initialize Auth Handler
	authHandler := NewAuthHandler(eng.GetPostgresClient().DB, cfg)
	
	// Create default admin user if missing
	authHandler.EnsureAdminUser()

	// Public Auth Routes (No Middleware)
	authGroup := app.Group("/auth")
	authGroup.Post("/register", authHandler.Register)
	authGroup.Post("/login", authHandler.Login)

	// Subscription Routes
	api := app.Group("/api")
	
	// Apply RBAC Middleware to /api
	// Pass JWT Secret to middleware
	// Note: For now we use the same hardcoded secret "hhwtrade-secret-key-2025" used in AuthHandler.
	// In production both should come from config.
	api.Use(middleware.CasbinMiddleware(enforcer, "hhwtrade-secret-key-2025"))
	api.Get("/users/:userID/subscriptions", subHandler.GetSubscriptions)
	api.Post("/users/:userID/subscriptions", subHandler.AddSubscription)
	api.Put("/users/:userID/subscriptions/reorder", subHandler.ReorderSubscriptions)
	api.Delete("/users/:userID/subscriptions/:symbol", subHandler.RemoveSubscription)

	api.Get("/futures-contracts/search", subHandler.SearchInstruments)
	api.Post("/futures-contracts/sync", subHandler.SyncInstruments)

	// Strategy Routes
	api.Post("/strategies", strategyHandler.CreateStrategy)
	api.Get("/users/:userID/strategies", strategyHandler.GetStrategies)
	api.Post("/strategies/:id/stop", strategyHandler.StopStrategy)
	api.Post("/strategies/:id/start", strategyHandler.StartStrategy)

	// Trade Routes
	tradeHandler := NewTradeHandler(eng)
	api.Post("/trade/order", tradeHandler.InsertOrder)
	api.Post("/trade/order/:id/cancel", tradeHandler.CancelOrder)
	api.Get("/users/:userID/positions", tradeHandler.GetPositions)
	api.Get("/users/:userID/orders", tradeHandler.GetOrders)
	api.Post("/users/:userID/sync-positions", tradeHandler.SyncPositions)
	api.Post("/users/:userID/sync-account", tradeHandler.SyncAccount)

	// Protected Auth Routes
	api.Get("/auth/me", authHandler.GetMe)
	api.Post("/auth/logout", authHandler.Logout)
	
	return app
}
