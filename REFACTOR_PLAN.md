# 架构优化方案

## 当前问题

### 1. 消息流过于复杂
- CTP → Redis → Engine → Handler → DB → WebSocket (6层)
- 每层都有序列化/反序列化开销
- 难以追踪和调试

### 2. Engine 职责过重
```go
type Engine struct {
    cfg          *config.Config
    pg           *infra.PostgresClient
    rdb          *redis.Client
    websocketHub *infra.WsManager
    subs         *SubscriptionState
    stratExec    *strategies.Executor
    ctpClient    *ctp.Client
    ctpHandler   *ctp.Handler
}
```
违反了单一职责原则

### 3. 命名不一致
- `WsManager` vs `websocketHub`
- `SubscriptionState` vs `subs`
- `stratExec` vs `Executor`

## 优化方案

### 方案 A: 事件总线模式（推荐）

```
┌─────────────────────────────────────┐
│         EventBus (核心)              │
│  - 统一的事件分发中心                 │
│  - 所有组件通过事件通信               │
└─────────────────────────────────────┘
         ↓           ↓           ↓
    ┌────────┐  ┌────────┐  ┌────────┐
    │ Market │  │ Trade  │  │Strategy│
    │Service │  │Service │  │Service │
    └────────┘  └────────┘  └────────┘
```

**优点：**
- 解耦各个模块
- 易于扩展新功能
- 统一的错误处理
- 便于测试

**实现：**
```go
// 事件定义
type Event struct {
    Type    EventType
    Source  string
    Data    interface{}
    Time    time.Time
}

type EventType string
const (
    EventMarketData   EventType = "market.data"
    EventOrderUpdate  EventType = "order.update"
    EventTradeUpdate  EventType = "trade.update"
    EventStrategySignal EventType = "strategy.signal"
)

// 事件总线
type EventBus struct {
    subscribers map[EventType][]chan Event
    mu          sync.RWMutex
}

func (eb *EventBus) Subscribe(eventType EventType) <-chan Event
func (eb *EventBus) Publish(event Event)
```

### 方案 B: 领域驱动设计（DDD）

```
fiber_api/
├── domain/              # 领域层（核心业务逻辑）
│   ├── market/         # 行情领域
│   │   ├── service.go
│   │   └── repository.go
│   ├── trading/        # 交易领域
│   │   ├── service.go
│   │   └── repository.go
│   └── strategy/       # 策略领域
│       ├── service.go
│       └── repository.go
├── application/        # 应用层（用例编排）
│   ├── market_app.go
│   ├── trading_app.go
│   └── strategy_app.go
├── infrastructure/     # 基础设施层
│   ├── ctp/
│   ├── database/
│   ├── redis/
│   └── websocket/
└── interfaces/         # 接口层
    ├── api/
    └── ws/
```

### 方案 C: 微服务化（长期）

将单体拆分为独立服务：
- Market Service (行情服务)
- Trading Service (交易服务)
- Strategy Service (策略服务)
- Gateway Service (API网关)

## 立即可执行的优化

### 1. 重命名优化

```go
// 之前
type WsManager struct { ... }
var websocketHub *infra.WsManager

// 之后
type Hub struct { ... }
var hub *websocket.Hub
```

### 2. 简化 Engine

```go
// 之前：Engine 做所有事情
type Engine struct {
    cfg, pg, rdb, websocketHub, subs, stratExec, ctpClient, ctpHandler
}

// 之后：Engine 只做协调
type Engine struct {
    marketService   *market.Service
    tradingService  *trading.Service
    strategyService *strategy.Service
}
```

### 3. 统一错误处理

```go
// 定义统一的错误类型
type AppError struct {
    Code    string
    Message string
    Cause   error
}

// 统一的错误处理中间件
func ErrorHandler() fiber.Handler {
    return func(c *fiber.Ctx) error {
        err := c.Next()
        if err != nil {
            // 统一处理和记录
            return handleError(c, err)
        }
        return nil
    }
}
```

### 4. 配置管理优化

```go
// 之前：配置散落各处
cfg.Server.Port
cfg.Database.Host

// 之后：使用配置对象
type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    Redis    RedisConfig
    CTP      CTPConfig  // 新增
}

// 支持环境变量覆盖
// 支持热重载
```

### 5. 日志优化

```go
// 使用结构化日志
import "go.uber.org/zap"

logger.Info("订单创建",
    zap.String("order_id", orderID),
    zap.String("instrument", instrumentID),
    zap.Float64("price", price),
)

// 而不是
log.Printf("Order created: %s, %s, %.2f", orderID, instrumentID, price)
```

## 具体重构步骤

### 第一阶段：重命名和整理（1-2天）
1. 统一命名规范
2. 整理目录结构
3. 提取常量和配置

### 第二阶段：职责分离（3-5天）
1. 拆分 Engine 为多个 Service
2. 引入 EventBus
3. 简化消息流

### 第三阶段：优化性能（2-3天）
1. 减少序列化开销
2. 使用连接池
3. 优化数据库查询

### 第四阶段：完善测试（持续）
1. 单元测试
2. 集成测试
3. 压力测试

## 命名规范建议

### 包命名
- `websocket` 而不是 `infra`
- `market` 而不是 `ctp`
- `persistence` 而不是 `model`

### 类型命名
- `Hub` 而不是 `WsManager`
- `Client` 而不是 `WsClient`
- `Subscription` 而不是 `SubscriptionState`

### 方法命名
- `Subscribe(symbol string)` 而不是 `AddSubscription`
- `Unsubscribe(symbol string)` 而不是 `RemoveSubscription`
- `Broadcast(msg Message)` 保持不变（已经很好）

### 变量命名
- `hub` 而不是 `websocketHub`
- `db` 而不是 `pg`
- `cache` 而不是 `rdb`

## 性能优化建议

### 1. 减少 Redis 往返
```go
// 之前：每次都序列化
json.Marshal(cmd)
rdb.LPush(ctx, queue, jsonStr)

// 之后：批量处理
pipe := rdb.Pipeline()
for _, cmd := range commands {
    pipe.LPush(ctx, queue, cmd)
}
pipe.Exec(ctx)
```

### 2. 使用对象池
```go
var messagePool = sync.Pool{
    New: func() interface{} {
        return &Message{}
    },
}

msg := messagePool.Get().(*Message)
defer messagePool.Put(msg)
```

### 3. WebSocket 优化
```go
// 使用二进制协议而不是 JSON
// 使用消息压缩
conn.EnableWriteCompression(true)
```

## 监控和可观测性

### 添加指标收集
```go
import "github.com/prometheus/client_golang/prometheus"

var (
    orderCounter = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "orders_total",
            Help: "Total number of orders",
        },
        []string{"status", "instrument"},
    )
)
```

### 添加链路追踪
```go
import "go.opentelemetry.io/otel"

ctx, span := tracer.Start(ctx, "ProcessOrder")
defer span.End()
```

## 总结

**立即优化（本周）：**
1. ✅ 重命名关键类型和变量
2. ✅ 提取配置常量
3. ✅ 添加结构化日志

**短期优化（本月）：**
1. 引入 EventBus 解耦
2. 拆分 Engine 职责
3. 优化错误处理

**长期规划（季度）：**
1. 考虑 DDD 架构
2. 添加完整测试
3. 性能调优和监控
