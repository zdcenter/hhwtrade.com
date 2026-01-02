# WebSocket 工作流程文档

本文档详细描述了 Angular 客户端连接 WebSocket 到 Go 服务的完整工作流程，包括数据结构变化、CTP 行情推送机制等。

---

## 1. 核心数据结构

### 1.1 WsClient - 单个 WebSocket 连接封装

```go
// 位置: internal/infra/websocket.go

type WsClient struct {
    conn   *websocket.Conn      // 底层 WebSocket 连接
    sendCh chan interface{}     // 写消息的缓冲通道 (容量 256)
}
```

**关键特性：**
- `sendCh` 是一个带缓冲的通道，避免业务逻辑直接调用 `WriteJSON` 导致阻塞
- 每个客户端创建时会启动一个独立的 `writeLoop` 协程处理消息发送
- 缓冲区满时会丢弃消息（对实时行情来说，丢弃旧数据比阻塞更好）

### 1.2 WsManager - WebSocket 管理器 (Hub 模式)

```go
// 位置: internal/infra/websocket.go

type WsManager struct {
    // 所有活跃的客户端
    clients map[*WsClient]bool

    mu sync.RWMutex        // 保护并发读写

    Register   chan *WsClient  // 注册通道
    Unregister chan *WsClient  // 注销通道
}
```

**一层映射关系：**
```
clients:       存储所有连接（用于全局广播）
```

### 1.3 MarketMessage - 行情消息

```go
// 位置: internal/infra/redis_pubsub.go

type MarketMessage struct {
    Symbol  string          // 合约代码，如 "rb2505"（内部路由用）
    Payload json.RawMessage // CTP 原始 JSON 数据
}

// 全局行情数据通道 (容量 10000)
var MarketDataChan = make(chan MarketMessage, 10000)
```

---

## 2. Angular 客户端连接流程

### 2.1 连接建立时序图

```
Angular                    Go WebSocket Handler           WsManager
   |                              |                           |
   |--- ws://host/ws ------------>|                           |
   |                              |                           |
   |                              |-- NewWsClient(conn) ----->|
   |                              |   创建 WsClient           |
   |                              |   启动 writeLoop 协程     |
   |                              |                           |
   |                              |-- Register 通道 --------->|
   |                              |   Client                  |
   |                              |                           |
   |                              |                    更新数据结构:
   |                              |                    clients[client] = true
   |                              |                           |
   |<---- 连接建立确认 -----------|                           |
```

### 2.2 数据结构变化示例

**初始状态（无连接）：**
```go
clients:       {}
```

**一个连接建立后：**
```go
clients:       {client1: true}
```

**再建立第二个连接：**
```go
clients:       {client1: true, client2: true}
```

---

## 3. 客户端订阅行情流程

### 3.1 订阅请求格式

```json
{
    "Action": "subscribe",
    "InstrumentID": "rb2505"
}
```

### 3.2 订阅时序图

```
Angular                WsHandler
   |                      |                            |                        |
   |-- subscribe rb2505 ->|                            |                        |
   |                      |                            |                        |
说明：在“全量广播”模型下，WebSocket 的 subscribe/unsubscribe 消息会被服务端忽略。
全局订阅由 HTTP 订阅管理接口（写入 DB）驱动；只要某合约被全局订阅，其行情到达后会广播给所有连接。
```

### 3.3 数据结构变化

**订阅 "rb2505" 后（全局订阅，不影响 WsManager 连接结构）：**
```go
clients:       {client1: true}
```

**client2 也连接后：**
```go
clients:       {client1: true, client2: true}
```

**全局再订阅 "ag2506"（不影响 WsManager 连接结构）：**
```go
clients:       {client1: true, client2: true}
```

---

## 4. CTP 行情推送到 Angular 的完整流程

### 4.1 数据流向图

```
┌─────────────┐     ┌─────────────┐     ┌──────────────────┐
│  CTP Core   │────>│    Redis    │────>│  Go 服务 (Engine) │
│  (Python)   │     │  Pub/Sub    │     │                  │
└─────────────┘     └─────────────┘     └────────┬─────────┘
                                                 │
                                                 ▼
                                        ┌──────────────────┐
                                        │    WsManager     │
                                        │   Broadcast()    │
                                        └────────┬─────────┘
                                                 │
                    ┌────────────────────────────┼────────────────────────────┐
                    │                            │                            │
                    ▼                            ▼                            ▼
            ┌──────────────┐            ┌──────────────┐            ┌──────────────┐
            │   Angular    │            │   Angular    │            │   Angular    │
            │   Client 1   │            │   Client 2   │            │   Client 3   │
            └──────────────┘            └──────────────┘            └──────────────┘
```

### 4.2 详细步骤说明

#### 步骤 1: CTP Core 发布行情
```python
# CTP Core (Python) 发布到 Redis
redis.publish("ctp:market:rb2505", json_data)
```

#### 步骤 2: Go Redis 订阅器接收
```go
// internal/infra/redis_pubsub.go - StartMarketDataSubscriber()

// 订阅模式: ctp:market:*
pattern := constants.RedisPubSubMarketPrefix + "*"  // "ctp:market:*"
pubsub := rdb.PSubscribe(ctx, pattern)

// 接收消息
for msg := range ch {
    // 从通道名提取合约代码
    // msg.Channel = "ctp:market:rb2505" -> symbol = "rb2505"
    symbol := strings.TrimPrefix(msg.Channel, constants.RedisPubSubMarketPrefix)

    // 构造内部消息
    message := MarketMessage{
        Symbol:  symbol,                      // "rb2505"
        Payload: json.RawMessage(msg.Payload), // CTP 原始 JSON
    }

    // 发送到全局通道
    MarketDataChan <- message
}
```

#### 步骤 3: MarketDataDispatcher 分发 (新架构)
```go
// internal/infra/dispatcher.go - Start()

func (d *MarketDataDispatcher) Start() {
    for msg := range MarketDataChan {
        // 1. 直接广播给 WebSocket 客户端 (UI)
        d.wsManager.Broadcast(msg)

        // 2. 传递给 Engine 进行策略计算 (Strategy)
        d.safeCallEngine(msg)
    }
}
```

**变化说明**:
- `Engine` 不再负责行情转发，只专注于策略逻辑。
- `MarketDataDispatcher` 作为新的枢纽，并行/串行分发数据。
- 只有策略相关的逻辑才会进入 `Engine.OnMarketData`。

#### 步骤 4: WsManager 广播
```go
// internal/infra/websocket.go - Broadcast()

func (m *WsManager) Broadcast(msg MarketMessage) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    // 向每个连接发送（全量广播）
    for client := range m.clients {
        client.Send(msg.Payload)  // 发送原始 JSON
    }
}
```

#### 步骤 5: WsClient 发送
```go
// internal/infra/websocket.go - Send() 和 writeLoop()

func (c *WsClient) Send(msg interface{}) {
    select {
    case c.sendCh <- msg:  // 放入缓冲通道
    default:
        log.Println("WS Warning: Client buffer full, dropping message")
    }
}

func (c *WsClient) writeLoop() {
    for {
        select {
        case msg, ok := <-c.sendCh:
            if !ok {
                return
            }
            c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
            c.conn.WriteJSON(msg)  // 实际发送到 Angular
        }
    }
}
```

### 4.3 CTP 行情数据格式

```json
{
    "InstrumentID": "rb2505",
    "ExchangeID": "SHFE",
    "LastPrice": 3850.0,
    "PreSettlementPrice": 3840.0,
    "PreClosePrice": 3845.0,
    "OpenPrice": 3842.0,
    "HighestPrice": 3865.0,
    "LowestPrice": 3838.0,
    "Volume": 125680,
    "Turnover": 4825468000.0,
    "OpenInterest": 1458620.0,
    "BidPrice1": 3849.0,
    "BidVolume1": 156,
    "AskPrice1": 3850.0,
    "AskVolume1": 203,
    "UpdateTime": "14:35:28",
    "UpdateMillisec": 500
}
```

---

## 5. 连接断开清理流程

### 5.1 清理时序图

```
Angular                    Go WsHandler              WsManager
   |                              |                      |
   |--- 连接断开 ---------------->|                      |
   |                              |                      |
   |                    defer cleanup                    |
   |                              |                      |
   |                              |-- Unregister 通道 -->|
   |                              |                      |
   |                              |               清理操作:
   |                              |               1. delete(clients, client)
   |                              |               2. client.Close()
   |                              |                      |
   |                              |-- MarketSvc.Unsubscribe() (可选)
   |                              |                      |
```

### 5.2 数据结构变化

**清理前：**
```go
clients:       {client1: true, client2: true}
```

**client1 断开后：**
```go
clients:       {client2: true}
```

---

## 6. 交易回报/系统事件推送

在单交易账号 + 全局订阅模型下：

- 订单/成交/错误等交易回报（由 `ctp.Handler` 处理）会通过 `WsManager.BroadcastToAll()` 广播给所有连接。
- 如果未来需要按用户隔离推送，可再引入 userConns，但当前架构选择保持简单。

---

## 7. 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Angular 客户端                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                       │
│  │   Browser 1  │  │   Browser 2  │  │   Browser 3  │                       │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘                       │
└─────────┼─────────────────┼─────────────────┼───────────────────────────────┘
          │ WebSocket       │ WebSocket       │ WebSocket
          └─────────────────┼─────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Go Fiber API 服务                               │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         WsManager (Hub)                              │   │
│  │  clients: map[*WsClient]bool                                        │   │
│  │  (全量广播模型：不维护 subscriptions)                                │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                            ▲                                                 │
│                            │ Broadcast()                                     │
│                            ▲                                                 │
│                            │ OnMarketData()                                  │
│  ┌─────────────────────────┴───────────────────────────────────────────┐   │
│  │                      MarketDataDispatcher                        │   │
│  │  - Start()              从 MarketDataChan 接收                       │   │
│  │  - Broadcast()          直接调用 WsManager                           │   │
│  │  - OnMarketData()       调用 Engine                                  │   │
│  └─────────────────────────┬───────────────────────────────────────────┘   │
│                            │ MarketDataChan                                  │
│  ┌─────────────────────────┴───────────────────────────────────────────┐   │
│  │                    Redis Pub/Sub Subscriber                          │   │
│  │  - StartMarketDataSubscriber()                                       │   │
│  │  - 订阅 ctp:market:* 模式                                             │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                            ▲                                                 │
└────────────────────────────┼────────────────────────────────────────────────┘
                            │ Redis Pub/Sub
                            ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                               Redis                                          │
│  Channel: ctp:market:rb2505, ctp:market:ag2506, ...                         │
└────────────────────────────┬────────────────────────────────────────────────┘
                            │ Publish
                            ▲
┌────────────────────────────┴────────────────────────────────────────────────┐
│                           CTP Core (Python)                                  │
│  - 接收交易所行情                                                             │
│  - 发布到 Redis                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 8. 关键设计决策

### 8.1 为什么使用 Channel 缓冲？
- **WsClient.sendCh (256)**：避免单个慢客户端阻塞整个广播
- **MarketDataChan (10000)**：应对行情高峰，防止 Redis 订阅器阻塞

### 8.2 为什么使用 Hub 模式？
- 集中管理所有连接状态
- 避免并发问题
- 方便实现按主题广播

### 8.3 为什么使用 json.RawMessage？
- 避免重复序列化/反序列化
- CTP 数据直接透传给前端
- 性能更好

---

## 9. 文件位置索引

| 文件 | 内容 |
|------|------|
| `internal/infra/websocket.go` | WsClient, WsManager 定义 |
| `internal/infra/redis_pubsub.go` | Redis 订阅器，MarketMessage 定义 |
| `internal/engine/engine.go` | 行情分发循环，协调器 |
| `internal/api/ws_handler.go` | WebSocket HTTP 端点处理 |
| `internal/constants/constants.go` | Redis 通道名称常量 |
| `cmd/main.go` | 服务启动入口 |
