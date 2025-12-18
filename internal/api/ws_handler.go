package api

import (
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/engine"
	"hhwtrade.com/internal/infra"
	"hhwtrade.com/internal/model"
)

type WsRequest struct {
	Type   string `json:"type"`   // "subscribe", "unsubscribe"
	Symbol string `json:"symbol"` // rb2601, "AAPL", "MSFT"
}

func InitWebsocket(app *fiber.App, eng *engine.Engine) {
	// Middleware to force upgrade
	// 确保请求头升级到 WebSocket
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	// Get WsManager from Engine
	wsManager := eng.GetWsManager()

	// WebSocket Endpoint
	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		// Get userID from Query Param (e.g. ?userID=101)
		userID := c.Query("userID")
		log.Println("New WS connection, userID:", userID)

		// 1. Register new connection with UserID
		wsManager.Register <- infra.UserConnection{Conn: c, UserID: userID}

		// 2. Cleanup on exit
		defer func() {
			wsManager.Unregister <- infra.UserConnection{Conn: c, UserID: userID}
			c.Close()
		}()

		// 3. Auto-subscribe stored symbols (Restore session)
		if userID != "" {
			go func() {
				var subs []model.UserSubscription
				db := eng.GetPostgresClient().DB
				if err := db.Where("user_id = ?", userID).Find(&subs).Error; err == nil {
					for _, sub := range subs {
						log.Printf("Auto-subscribing %s to %s", userID, sub.Symbol)
						wsManager.Subscribe <- infra.Subscription{Conn: c, Symbol: sub.Symbol}
					}
				} else {
					log.Printf("Failed to fetch subscriptions for user %s: %v", userID, err)
				}
			}()
		}

		// 4. Read Loop
		var (
			msg WsRequest
			err error
		)
		for {
			if err = c.ReadJSON(&msg); err != nil {
				// Error or Close
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Println("ws read error:", err)
				}
				break // Exit loop to trigger cleanup
			}

			// Handle Actions
			// 使用 tagged switch 来处理 msg.Type
			switch msg.Type {
			case "subscribe":
				// 处理订阅逻辑
				wsManager.Subscribe <- infra.Subscription{Conn: c, Symbol: msg.Symbol}

				// 可选：通知引擎进行订阅（例如订阅 Redis）
				// eng.GetSubscriptionState().AddSubscription(msg.Topic)
				// 如果引擎没有监听 Redis，则应订阅。当前 StartMarketDataSubscriber 已经订阅了 "market.*"。
				// 所以暂时不需要动态的 Redis 订阅。

			case "unsubscribe":
				// 处理取消订阅逻辑
				wsManager.Unsubscribe <- infra.Subscription{Conn: c, Symbol: msg.Symbol}

			default:
				// 处理未知类型的情况
				log.Println("Unexpected type:", msg.Type)
			}
		}
	}))
}
