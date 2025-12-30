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
	Action       string `json:"Action"`
	InstrumentID string `json:"InstrumentID"`
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

		// 1. Create Client Wrapper
		client := infra.NewWsClient(c)

		// 2. Register
		wsManager.Register <- &infra.RegisterReq{
			Client: client,
			UserID: userID,
		}

		// Track which symbols this connection has subscribed to for cleanup
		localSubs := make(map[string]bool)

		// 3. Cleanup on exit
		defer func() {
			wsManager.Unregister <- client
			// Engine cleanup: Unsubscribe from all symbols this connection was watching
			for instrumentID := range localSubs {
				if err := eng.UnsubscribeSymbol(instrumentID); err != nil {
					log.Printf("WS Cleanup: Failed to unsubscribe %s: %v", instrumentID, err)
				}
			}
			// Client Close is handled by Unregister -> client.Close()
		}()

		// 4. Auto-subscribe stored symbols (Restore session)
		if userID != "" {
			go func() {
				var subs []model.Subscription
				db := eng.GetPostgresClient().DB
				if err := db.Where("user_id = ?", userID).Find(&subs).Error; err == nil {
					for _, sub := range subs {
						log.Printf("Auto-subscribing %s to %s", userID, sub.InstrumentID)
						wsManager.Subscribe(client, sub.InstrumentID)
						
						// Mark for local tracking and trigger Engine subscription
						localSubs[sub.InstrumentID] = true
						if err := eng.SubscribeSymbol(sub.InstrumentID); err != nil {
							log.Printf("WS Auto-sub: Failed to trigger CTP subscription for %s: %v", sub.InstrumentID, err)
						}
					}
				} else {
					log.Printf("Failed to fetch subscriptions for user %s: %v", userID, err)
				}
			}()
		}

		// 5. Read Loop
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
			switch msg.Action {
			case "subscribe":
				// Handle logical subscription
				wsManager.Subscribe(client, msg.InstrumentID)
				
				// Track locally and trigger engine
				if !localSubs[msg.InstrumentID] {
					localSubs[msg.InstrumentID] = true
					if err := eng.SubscribeSymbol(msg.InstrumentID); err != nil {
						log.Printf("WS: Failed to trigger CTP subscription for %s: %v", msg.InstrumentID, err)
					}
				}

			case "unsubscribe":
				// Handle logical unsubscription
				wsManager.Unsubscribe(client, msg.InstrumentID)
				
				// Remove from local tracking and trigger engine
				if localSubs[msg.InstrumentID] {
					delete(localSubs, msg.InstrumentID)
					if err := eng.UnsubscribeSymbol(msg.InstrumentID); err != nil {
						log.Printf("WS: Failed to trigger CTP unsubscription for %s: %v", msg.InstrumentID, err)
					}
				}

			default:
				log.Println("Unexpected type:", msg.Action)
			}
		}
	}))
}
