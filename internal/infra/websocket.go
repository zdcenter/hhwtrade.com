package infra

import (
	"log"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
)

// WsClient 封装单个 WebSocket 连接
// 负责维护该连接的写队列，确保线程安全
type WsClient struct {
	// 底层连接
	conn *websocket.Conn

	// 写消息的缓冲通道
	// 避免直接在业务逻辑中调用 WriteJSON 导致阻塞
	sendCh chan interface{}
}

// NewWsClient 创建新的客户端实例并启动写循环
func NewWsClient(conn *websocket.Conn) *WsClient {
	c := &WsClient{
		conn:   conn,
		sendCh: make(chan interface{}, 256), // 256 是缓冲区大小，防止消息积压
	}
	go c.writeLoop()
	return c
}

// writeLoop 是一个常驻协程，专门处理发往该客户端的消息
// 这样可以确保同一个 Conn 的 Write 操作是串行的
func (c *WsClient) writeLoop() {
	defer func() {
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.sendCh:
			if !ok {
				// 通道被关闭，说明连接已断开
				return
			}
			// 设置写超时，防止网络卡死
			c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteJSON(msg); err != nil {
				log.Printf("WS Error: %v", err)
				return // 发生错误，退出循环，触发 Close
			}
		}
	}
}

// Send 发送消息给客户端（非阻塞，除非缓冲已满）
func (c *WsClient) Send(msg interface{}) {
	select {
	case c.sendCh <- msg:
	default:
		// 缓冲区已满，直接丢弃或记录日志
		// 对于实时行情，丢弃旧数据通常比阻塞好
		log.Println("WS Warning: Client buffer full, dropping message")
	}
}

// Close 关闭客户端连接
func (c *WsClient) Close() {
	close(c.sendCh)
}

// -------------------------------------------------------------

// WsManager 管理所有的 WebSocket 客户端连接和订阅关系
// 采用 Hub 模式：所有的注册、注销、广播都由 Manager 统一调度
type WsManager struct {
	// 所有活跃的客户端集合
	// map[*WsClient]bool
	clients map[*WsClient]bool

	// 订阅表: Symbol -> 客户端集合
	// map[string]map[*WsClient]bool
	subscriptions map[string]map[*WsClient]bool

	// 用户映射: UserID -> 客户端集合 (一个用户可能开多个网页)
	// map[string]map[*WsClient]bool
	userConns map[string]map[*WsClient]bool

	// 互斥锁，保护上述 map 的并发读写
	mu sync.RWMutex

	// 注册通道
	Register chan *RegisterReq
	// 注销通道
	Unregister chan *WsClient
}

// RegisterReq 注册请求结构体
type RegisterReq struct {
	Client *WsClient
	UserID string // 可选，用于私有推送
}

// NewWsManager 创建管理器
func NewWsManager() *WsManager {
	return &WsManager{
		clients:       make(map[*WsClient]bool),
		subscriptions: make(map[string]map[*WsClient]bool),
		userConns:     make(map[string]map[*WsClient]bool),
		Register:      make(chan *RegisterReq),
		Unregister:    make(chan *WsClient),
	}
}

// Start 启动管理器的事件循环
func (m *WsManager) Start() {
	log.Println("WebSocket Manager Started (Refactored)")
	for {
		select {
		case req := <-m.Register:
			m.mu.Lock()
			m.clients[req.Client] = true
			if req.UserID != "" {
				if m.userConns[req.UserID] == nil {
					m.userConns[req.UserID] = make(map[*WsClient]bool)
				}
				m.userConns[req.UserID][req.Client] = true
			}
			m.mu.Unlock()
			log.Printf("WS: New client registered (User: %s)", req.UserID)

		case client := <-m.Unregister:
			m.mu.Lock()
			if _, ok := m.clients[client]; ok {
				delete(m.clients, client)
				client.Close()

				// 清理用户映射
				for userID, conns := range m.userConns {
					delete(conns, client)
					if len(conns) == 0 {
						delete(m.userConns, userID)
					}
				}

				// 清理订阅
				for symbol, subscribers := range m.subscriptions {
					delete(subscribers, client)
					if len(subscribers) == 0 {
						delete(m.subscriptions, symbol)
					}
				}
			}
			m.mu.Unlock()
			log.Println("WS: Client unregistered")
		}
	}
}

// Subscribe 客户端订阅某个 Topic
func (m *WsManager) Subscribe(client *WsClient, symbol string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.subscriptions[symbol] == nil {
		m.subscriptions[symbol] = make(map[*WsClient]bool)
	}
	m.subscriptions[symbol][client] = true
}

// Unsubscribe 客户端取消订阅
func (m *WsManager) Unsubscribe(client *WsClient, symbol string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if clients, ok := m.subscriptions[symbol]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(m.subscriptions, symbol)
		}
	}
}

// Broadcast 广播行情数据给所有订阅者
func (m *WsManager) Broadcast(msg MarketMessage) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	subscribers, ok := m.subscriptions[msg.Symbol]
	if !ok {
		return
	}

	for client := range subscribers {
		client.Send(msg.Payload)
	}
}

// PushToUser 推送私有消息给特定用户
func (m *WsManager) PushToUser(userID string, msg interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	clients, ok := m.userConns[userID]
	if !ok {
		return
	}

	for client := range clients {
		client.Send(msg)
	}
}

// SubscribeUser 为指定用户的当前所有活跃连接订阅 Symbol
func (m *WsManager) SubscribeUser(userID, symbol string) {
	// 1. 获取用户当前所有的连接 (RLock)
	m.mu.RLock()
	var targetClients []*WsClient
	if clients, ok := m.userConns[userID]; ok {
		for client := range clients {
			targetClients = append(targetClients, client)
		}
	}
	m.mu.RUnlock()

	// 2. 逐个订阅 (Lock)
	// 必须分开操作，避免死锁 (Subscribe 内部会加 Lock)
	for _, client := range targetClients {
		m.Subscribe(client, symbol)
	}
}

// UnsubscribeUser 为指定用户取消订阅 Symbol
func (m *WsManager) UnsubscribeUser(userID, symbol string) {
	m.mu.RLock()
	var targetClients []*WsClient
	if clients, ok := m.userConns[userID]; ok {
		for client := range clients {
			targetClients = append(targetClients, client)
		}
	}
	m.mu.RUnlock()

	for _, client := range targetClients {
		m.Unsubscribe(client, symbol)
	}
}

// BroadcastMarketData 广播行情数据 (实现 domain.Notifier 接口)
func (m *WsManager) BroadcastMarketData(data interface{}) {
	if msg, ok := data.(MarketMessage); ok {
		m.Broadcast(msg)
	}
}
