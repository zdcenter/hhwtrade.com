package infra

import (
	"log"
	"sync"

	"github.com/gofiber/contrib/websocket"
)

// WsManager manages WebSocket connections and subscriptions.
type WsManager struct {
	// Active clients
	// map[连接对象的内存地址]存在
	// {
	//     0xc0000a0100: true,  // -> 这是一个来自张三的 WebSocket 连接对象
	//     0xc0000a0280: true,  // -> 这是一个来自李四的 WebSocket 连接对象
	//     0xc0000a0400: true,  // -> 这是一个来自王五的 WebSocket 连接对象
	// }
	clients map[*websocket.Conn]bool

	// map[主题]map[连接对象的内存地址]存在
	// {
	// 	// 品种 "rb2601" (螺纹钢): 张三和李四都订阅了
	// 	"rb2601": {
	// 		0xc0000a0100: true, // -> 张三
	// 		0xc0000a0280: true, // -> 李四
	// 	},
	// 	// 品种 "BTC-USDT" (比特币): 只有王五订阅了
	// 	"BTC-USDT": {
	// 		0xc0000a0400: true, // -> 王五
	// 	},
	// 	// 品种 "AAPL" (苹果): 张三也订阅了这个
	// 	"AAPL": {
	// 		0xc0000a0100: true, // -> 张三 (一个人可以订阅多个)
	// 	}
	// }
	subscriptions map[string]map[*websocket.Conn]bool

	// Mutex to protect maps
	mu sync.RWMutex

	// Channels for actions
	Register    chan UserConnection
	Unregister  chan UserConnection
	Subscribe   chan Subscription
	Unsubscribe chan Subscription

	// User connection mapping: UserID -> Set of Connections
	userConns map[string]map[*websocket.Conn]bool

	// sendChannels stores a buffered channel for each client.
	// This helps avoid blocking the main engine loop if one client is slow.
	sendChannels map[*websocket.Conn]chan interface{}
}

type UserConnection struct {
	UserID string
	Conn   *websocket.Conn
}

type Subscription struct {
	Conn   *websocket.Conn
	Symbol string
}

var GlobalWsManager = &WsManager{
	clients:       make(map[*websocket.Conn]bool),
	userConns:     make(map[string]map[*websocket.Conn]bool),
	subscriptions: make(map[string]map[*websocket.Conn]bool),
	sendChannels:  make(map[*websocket.Conn]chan interface{}),
	Register:      make(chan UserConnection),
	Unregister:    make(chan UserConnection),
	Subscribe:     make(chan Subscription),
	Unsubscribe:   make(chan Subscription),
}

// SubscribeUser manually triggers subscription for a specific user ID.
// This is used by the HTTP API side.
func (manager *WsManager) SubscribeUser(userID, symbol string) {
	manager.mu.RLock()
	conns, ok := manager.userConns[userID]
	manager.mu.RUnlock()

	if ok {
		for conn := range conns {
			manager.Subscribe <- Subscription{Conn: conn, Symbol: symbol}
		}
	}
}

// UnsubscribeUser manually triggers unsubscription for a specific user ID.
func (manager *WsManager) UnsubscribeUser(userID, symbol string) {
	manager.mu.RLock()
	conns, ok := manager.userConns[userID]
	manager.mu.RUnlock()

	if ok {
		for conn := range conns {
			manager.Unsubscribe <- Subscription{Conn: conn, Symbol: symbol}
		}
	}
}

func (manager *WsManager) Start() {
	log.Println("Starting WebSocket Manager...")
	for {
		select {
		case req := <-manager.Register:
			manager.mu.Lock()
			manager.clients[req.Conn] = true

			// Create a buffered channel for this connection
			sendCh := make(chan interface{}, 256)
			manager.sendChannels[req.Conn] = sendCh

			// Start a dedicated writer goroutine for this connection
			go func(conn *websocket.Conn, ch chan interface{}) {
				for msg := range ch {
					if err := conn.WriteJSON(msg); err != nil {
						// On error, let the connection close and unregister handle it
						log.Printf("WS WriteLoop error: %v", err)
						conn.Close()
						return
					}
				}
			}(req.Conn, sendCh)

			// Track user connection
			if req.UserID != "" {
				if manager.userConns[req.UserID] == nil {
					manager.userConns[req.UserID] = make(map[*websocket.Conn]bool)
				}
				manager.userConns[req.UserID][req.Conn] = true
			}

			manager.mu.Unlock()
			log.Printf("New WebSocket client connected: %s", req.UserID)

		case req := <-manager.Unregister:
			manager.mu.Lock()
			if _, ok := manager.clients[req.Conn]; ok {
				delete(manager.clients, req.Conn)

				// Cleanup send channel
				if ch, exists := manager.sendChannels[req.Conn]; exists {
					close(ch)
					delete(manager.sendChannels, req.Conn)
				}

				// Remove from user mapping
				if req.UserID != "" && manager.userConns[req.UserID] != nil {
					delete(manager.userConns[req.UserID], req.Conn)
					if len(manager.userConns[req.UserID]) == 0 {
						delete(manager.userConns, req.UserID)
					}
				}

				// Remove from all subscriptions
				for topic, clients := range manager.subscriptions {
					delete(clients, req.Conn)
					if len(clients) == 0 {
						delete(manager.subscriptions, topic)
					}
				}
			}
			manager.mu.Unlock()
			log.Println("WebSocket client disconnected")

		case sub := <-manager.Subscribe:
			manager.mu.Lock()
			if manager.subscriptions[sub.Symbol] == nil {
				manager.subscriptions[sub.Symbol] = make(map[*websocket.Conn]bool)
			}
			manager.subscriptions[sub.Symbol][sub.Conn] = true
			manager.mu.Unlock()
			log.Printf("Client subscribed to %s", sub.Symbol)

		case sub := <-manager.Unsubscribe:
			manager.mu.Lock()
			if clients, ok := manager.subscriptions[sub.Symbol]; ok {
				delete(clients, sub.Conn)
				if len(clients) == 0 {
					delete(manager.subscriptions, sub.Symbol)
				}
			}
			manager.mu.Unlock()
			log.Printf("Client unsubscribed from %s", sub.Symbol)
		}
	}
}

// Broadcast sends the market message to all subscribers.
func (manager *WsManager) Broadcast(msg MarketMessage) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	subscribers, ok := manager.subscriptions[msg.Symbol]
	if ok {
		// Defensive check: don't broadcast empty payloads which break JSON marshaling
		if len(msg.Payload) == 0 {
			return
		}
		for conn := range subscribers {
			if ch, exists := manager.sendChannels[conn]; exists {
				select {
				case ch <- msg.Payload: // 只推送原始 Payload (json.RawMessage)
				default:
					// Buffer full: drop message for this specific slow client
				}
			}
		}
	}
}

// PushToUser sends a message to all active connections of a specific user.
func (manager *WsManager) PushToUser(userID string, msg interface{}) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	conns, ok := manager.userConns[userID]
	if ok {
		for conn := range conns {
			if ch, exists := manager.sendChannels[conn]; exists {
				select {
				case ch <- msg:
				default:
					// Skip if buffer is full
				}
			}
		}
	}
}
