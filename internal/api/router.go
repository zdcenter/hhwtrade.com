package api

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"hhwtrade.com/internal/api/middleware"
	"hhwtrade.com/internal/auth"
	"hhwtrade.com/internal/config"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/infra"
)

// Router 负责注册所有路由
type Router struct {
	app    *fiber.App
	cfg    *config.Config
	db     *gorm.DB
	wsHub  *infra.WsManager
	router fiber.Router // /api group

	// 服务层依赖
	subscriptionSvc domain.SubscriptionService
	tradingSvc      domain.TradingService
	strategySvc     domain.StrategyService
	marketSvc       domain.MarketService
}

// RouterDeps 路由器依赖
type RouterDeps struct {
	App             *fiber.App
	Cfg             *config.Config
	DB              *gorm.DB
	WsHub           *infra.WsManager
	SubscriptionSvc domain.SubscriptionService
	TradingSvc      domain.TradingService
	StrategySvc     domain.StrategyService
	MarketSvc       domain.MarketService
}

// NewRouter 创建路由器
func NewRouter(deps RouterDeps) *Router {
	return &Router{
		app:             deps.App,
		cfg:             deps.Cfg,
		db:              deps.DB,
		wsHub:           deps.WsHub,
		subscriptionSvc: deps.SubscriptionSvc,
		tradingSvc:      deps.TradingSvc,
		strategySvc:     deps.StrategySvc,
		marketSvc:       deps.MarketSvc,
	}
}

// RegisterRoutes 注册所有业务路由
func (r *Router) RegisterRoutes() {
	// 1. 初始化鉴权与中间件
	enforcer, err := auth.InitCasbin(r.db)
	if err != nil {
		log.Fatalf("Failed to initialize Casbin: %v", err)
	}

	// 2. 初始化各个 Handler (依赖接口)
	authHandler := NewAuthHandler(r.db, r.cfg)
	subHandler := NewSubscriptionHandler(r.subscriptionSvc)
	strategyHandler := NewStrategyHandler(r.strategySvc)
	futureHandler := NewFutureHandler(r.db, r.marketSvc)
	tradeHandler := NewTradeHandler(r.tradingSvc)

	// 3. 注册 WebSocket 路由 (不需要 JWT 中间件)
	InitWebsocketWithHub(r.app, r.wsHub)

	// 4. 注册公开路由 (Public)
	r.app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":  "ok",
			"message": "Service is healthy",
		})
	})

	// Auth Public Routes
	r.app.Post("/auth/register", authHandler.Register)
	r.app.Post("/auth/login", authHandler.Login)
	authHandler.EnsureAdminUser()

	// 5. 注册受保护的 API 路由 (Protected /api)
	r.router = r.app.Group("/api")
	jwtSecret := r.cfg.Server.JwtSecret	
	r.router.Use(middleware.CasbinMiddleware(enforcer, jwtSecret))

	// 分组注册子路由
	r.registerUserRoutes(subHandler, strategyHandler, tradeHandler)
	r.registerMarketRoutes(futureHandler)
	r.registerTradeRoutes(tradeHandler)
	r.registerStrategyRoutes(strategyHandler)
	r.registerAuthRoutes(authHandler)
}

func (r *Router) registerUserRoutes(sub *SubscriptionHandler, strat *StrategyHandler, trade *TradeHandler) {
	// Global Subscriptions
	r.router.Get("/subscriptions", sub.GetSubscriptions)
	r.router.Post("/subscriptions", sub.AddSubscription)
	r.router.Put("/subscriptions/reorder", sub.ReorderSubscriptions)
	r.router.Delete("/subscriptions/:symbol", sub.RemoveSubscription)

	users := r.router.Group("/users/:userID")

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
