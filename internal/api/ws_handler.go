package api

import (
	"context"
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/infra"
	"hhwtrade.com/internal/model"
)

type WsRequest struct {
	Action       string `json:"Action"`
	InstrumentID string `json:"InstrumentID"`
}

// WsHandlerDeps WebSocket 处理器依赖
type WsHandlerDeps struct {
	WsManager *infra.WsManager
	MarketSvc domain.MarketService
	DB        *gorm.DB
}

// InitWebsocketWithHub 使用依赖注入初始化 WebSocket
func InitWebsocketWithHub(app *fiber.App, wsManager *infra.WsManager) {
	// Middleware to force upgrade
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	// WebSocket Endpoint (简化版，不依赖 Engine)
	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		userID := c.Query("userID")
		log.Println("New WS connection, userID:", userID)

		// 1. Create Client Wrapper
		client := infra.NewWsClient(c)

		// 2. Register
		wsManager.Register <- &infra.RegisterReq{
			Client: client,
			UserID: userID,
		}

		// 3. Cleanup on exit
		defer func() {
			wsManager.Unregister <- client
		}()

		// 4. Read Loop
		var (
			msg WsRequest
			err error
		)
		for {
			if err = c.ReadJSON(&msg); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Println("ws read error:", err)
				}
				break
			}

			switch msg.Action {
			case "subscribe":
				wsManager.Subscribe(client, msg.InstrumentID)
			case "unsubscribe":
				wsManager.Unsubscribe(client, msg.InstrumentID)
			default:
				log.Println("Unexpected type:", msg.Action)
			}
		}
	}))
}

// InitWebsocketFull 完整版 WebSocket 初始化（支持行情订阅）
func InitWebsocketFull(app *fiber.App, deps WsHandlerDeps) {
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		userID := c.Query("userID")
		log.Println("New WS connection, userID:", userID)

		client := infra.NewWsClient(c)

		deps.WsManager.Register <- &infra.RegisterReq{
			Client: client,
			UserID: userID,
		}

		localSubs := make(map[string]bool)

		defer func() {
			deps.WsManager.Unregister <- client
			// 清理订阅
			for instrumentID := range localSubs {
				if err := deps.MarketSvc.Unsubscribe(context.Background(), instrumentID); err != nil {
					log.Printf("WS Cleanup: Failed to unsubscribe %s: %v", instrumentID, err)
				}
			}
		}()

		// 自动订阅用户保存的合约
		if userID != "" && deps.DB != nil {
			go func() {
				var subs []model.Subscription
				if err := deps.DB.Where("user_id = ?", userID).Find(&subs).Error; err == nil {
					for _, sub := range subs {
						log.Printf("Auto-subscribing %s to %s", userID, sub.InstrumentID)
						deps.WsManager.Subscribe(client, sub.InstrumentID)
						localSubs[sub.InstrumentID] = true
						if err := deps.MarketSvc.Subscribe(context.Background(), sub.InstrumentID); err != nil {
							log.Printf("WS Auto-sub: Failed to subscribe %s: %v", sub.InstrumentID, err)
						}
					}
				}
			}()
		}

		// Read Loop
		var msg WsRequest
		for {
			if err := c.ReadJSON(&msg); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Println("ws read error:", err)
				}
				break
			}

			switch msg.Action {
			case "subscribe":
				deps.WsManager.Subscribe(client, msg.InstrumentID)
				if !localSubs[msg.InstrumentID] {
					localSubs[msg.InstrumentID] = true
					if err := deps.MarketSvc.Subscribe(context.Background(), msg.InstrumentID); err != nil {
						log.Printf("WS: Failed to subscribe %s: %v", msg.InstrumentID, err)
					}
				}
			case "unsubscribe":
				deps.WsManager.Unsubscribe(client, msg.InstrumentID)
				if localSubs[msg.InstrumentID] {
					delete(localSubs, msg.InstrumentID)
					if err := deps.MarketSvc.Unsubscribe(context.Background(), msg.InstrumentID); err != nil {
						log.Printf("WS: Failed to unsubscribe %s: %v", msg.InstrumentID, err)
					}
				}
			default:
				log.Println("Unexpected type:", msg.Action)
			}
		}
	}))
}
