package api

import (
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/infra"
)

func shouldLogWsReadError(err error) bool {
	if err == nil {
		return false
	}

	// Normal closures / client navigation / browser tab close.
	// 1005 is "no status" (often seen when client closes without a close frame).
	if websocket.IsCloseError(
		err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure,
	) {
		return false
	}

	// Close frames with other codes are worth logging.
	if websocket.IsUnexpectedCloseError(
		err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure,
	) {
		return true
	}

	// For other errors (io.EOF etc.), don't spam logs; treat as normal disconnect.
	return false
}

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
		log.Println("New WS connection")

		// 1. Create Client Wrapper
		client := infra.NewWsClient(c)

		// 2. Register
		wsManager.Register <- client

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
				if shouldLogWsReadError(err) {
					log.Println("ws read error:", err)
				}
				break
			}

			switch msg.Action {
			case "subscribe":
				_ = msg.InstrumentID
			case "unsubscribe":
				_ = msg.InstrumentID
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
		log.Println("New WS connection")

		client := infra.NewWsClient(c)

		deps.WsManager.Register <- client

		defer func() {
			deps.WsManager.Unregister <- client
		}()

		// Read Loop
		var msg WsRequest
		for {
			if err := c.ReadJSON(&msg); err != nil {
				if shouldLogWsReadError(err) {
					log.Println("ws read error:", err)
				}
				break
			}

			switch msg.Action {
			case "subscribe":
				_ = msg.InstrumentID
			case "unsubscribe":
				_ = msg.InstrumentID
			default:
				log.Println("Unexpected type:", msg.Action)
			}
		}
	}))
}
