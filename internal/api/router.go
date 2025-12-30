package api

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/api/middleware"
	"hhwtrade.com/internal/auth"
	"hhwtrade.com/internal/config"
	"hhwtrade.com/internal/engine"
)

// Router 负责注册所有路由
type Router struct {
	app    *fiber.App
	cfg    *config.Config
	eng    *engine.Engine
	router fiber.Router // /api group
}

func NewRouter(app *fiber.App, cfg *config.Config, eng *engine.Engine) *Router {
	return &Router{
		app: app,
		cfg: cfg,
		eng: eng,
	}
}

// RegisterRoutes 注册所有业务路由
func (r *Router) RegisterRoutes() {
	// 1. 初始化鉴权与中间件
	// Initialize Casbin Enforcer
	enforcer, err := auth.InitCasbin(r.eng.GetPostgresClient().DB)
	if err != nil {
		log.Fatalf("Failed to initialize Casbin: %v", err)
	}

	// 2. 初始化各个 Handler
	authHandler := NewAuthHandler(r.eng.GetPostgresClient().DB, r.cfg)
	subHandler := NewSubscriptionHandler(r.eng)
	strategyHandler := NewStrategyHandler(r.eng)
	futureHandler := NewFutureHandler(r.eng)
	tradeHandler := NewTradeHandler(r.eng)

	// 3. 注册 WebSocket 路由 (不需要 JWT 中间件)
	InitWebsocket(r.app, r.eng)

	// 4. 注册公开路由 (Public)
	// Health Check
	r.app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":  "ok",
			"message": "Service is healthy",
		})
	})

	// Auth Public Routes
	r.app.Post("/auth/register", authHandler.Register)
	r.app.Post("/auth/login", authHandler.Login)
	authHandler.EnsureAdminUser() // Ensure admin exists

	// 5. 注册受保护的 API 路由 (Protected /api)
	r.router = r.app.Group("/api")
	// Apply RBAC/JWT Middleware
	// Note: For now we use the same hardcoded secret "hhwtrade-secret-key-2025" used in AuthHandler.
	jwtSecret := "hhwtrade-secret-key-2025" 
	r.router.Use(middleware.CasbinMiddleware(enforcer, jwtSecret))

	// 分组注册子路由
	r.registerUserRoutes(subHandler, strategyHandler, tradeHandler) // Subscription, Strategy, Trade (User-scoped)
	r.registerMarketRoutes(futureHandler)                           // Market Data logic
	r.registerTradeRoutes(tradeHandler)                             // Direct Trade actions
	r.registerStrategyRoutes(strategyHandler)                       // Strategy Management
	r.registerAuthRoutes(authHandler)                               // Me, Logout
}

func (r *Router) registerUserRoutes(sub *SubscriptionHandler, strat *StrategyHandler, trade *TradeHandler) {
	// User Sub-resources
	users := r.router.Group("/users/:userID")
	
	// Subscriptions
	users.Get("/subscriptions", sub.GetSubscriptions)
	users.Post("/subscriptions", sub.AddSubscription)
	users.Put("/subscriptions/reorder", sub.ReorderSubscriptions) // Note: this might need check if user ID matches param
	users.Delete("/subscriptions/:symbol", sub.RemoveSubscription)

	// Strategies
	users.Get("/strategies", strat.GetStrategies)

	// Positions & Orders
	users.Get("/positions", trade.GetPositions)
	users.Get("/orders", trade.GetOrders)
	users.Post("/sync-positions", trade.SyncPositions)
	users.Post("/sync-account", trade.SyncAccount)
}

func (r *Router) registerMarketRoutes(h *FutureHandler) {
	futures := r.router.Group("/futures")
	futures.Get("/", h.GetFutures)
	futures.Get("/search", h.SearchInstruments)
	futures.Post("/sync", h.SyncInstruments)
	futures.Post("/cleanup", h.CleanupExpired)
	futures.Get("/:id", h.GetFuture)
	futures.Put("/:id", h.UpdateFuture)
	futures.Delete("/:id", h.DeleteFuture)
}

func (r *Router) registerStrategyRoutes(h *StrategyHandler) {
	strategies := r.router.Group("/strategies")
	strategies.Post("/", h.CreateStrategy)
	strategies.Get("/:id", h.GetStrategy)
	strategies.Put("/:id", h.UpdateStrategy)
	strategies.Delete("/:id", h.DeleteStrategy)
	strategies.Post("/:id/stop", h.StopStrategy)
	strategies.Post("/:id/start", h.StartStrategy)
}

func (r *Router) registerTradeRoutes(h *TradeHandler) {
	trade := r.router.Group("/trade")
	trade.Post("/order", h.InsertOrder)
	trade.Post("/order/:id/cancel", h.CancelOrder)
}

func (r *Router) registerAuthRoutes(h *AuthHandler) {
	r.router.Get("/auth/me", h.GetMe)
	r.router.Post("/auth/logout", h.Logout)
}
