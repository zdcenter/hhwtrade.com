# 代码重构总结与建议

## ✅ 已完成的工作

### 1. 创建了新的架构组件

#### 常量包 (`internal/constants/`)
- ✅ `redis.go` - Redis 队列和频道名称
- ✅ `events.go` - 事件类型定义

#### 事件总线 (`internal/event/`)
- ✅ `bus.go` - 完整的事件总线实现
  - 异步/同步事件发布
  - 多订阅者支持
  - 并发安全
  - 优雅关闭

#### 服务层 (`internal/service/`)
- ✅ `market.go` - 行情服务
- ✅ `trading.go` - 交易服务
- ✅ `strategy.go` - 策略服务

### 2. 更新了现有代码

- ✅ 更新 `engine.go` 导入 constants 包
- ✅ 更新 `redis_pubsub.go` 使用 constants
- ✅ 移除 `ctp/types.go` 中的重复常量定义
- ✅ 更新 `ctp/client.go` 使用 constants

## ⚠️ 遇到的问题

### 1. 文件权限问题
某些文件无法直接写入，需要使用 sudo 或手动编辑

### 2. 编译错误
- `ctp.Client` 缺少 `GetRedisClient()` 方法
- 需要手动添加到 `client.go`

## 📋 剩余工作

### 立即需要完成的

1. **添加 GetRedisClient 方法到 ctp/client.go**
   ```go
   // 在文件末尾添加
   // GetRedisClient 返回 Redis 客户端（用于其他组件访问）
   func (c *Client) GetRedisClient() *redis.Client {
       return c.rdb
   }
   ```

2. **验证编译**
   ```bash
   cd /home/zd/ctp/fiber_api
   go build ./...
   ```

### 后续优化工作

#### 阶段 1: 集成新服务到现有 Engine（推荐先做）

在 `internal/engine/engine.go` 中：

```go
type Engine struct {
    // 现有字段
    cfg          *config.Config
    pg           *infra.PostgresClient
    rdb          *redis.Client
    websocketHub *infra.WsManager
    subs         *SubscriptionState
    stratExec    *strategies.Executor
    ctpClient    *ctp.Client
    ctpHandler   *ctp.Handler
    
    // 新增：事件总线和服务层
    eventBus        *event.Bus
    marketService   *service.MarketService
    tradingService  *service.TradingService
    strategyService *service.StrategyService
}
```

修改 `NewEngine` 函数：

```go
func NewEngine(cfg *config.Config, pg *infra.PostgresClient, rdb *redis.Client, wsHub *infra.WsManager) *Engine {
    // 1. 创建事件总线
    eventBus := event.NewBus(1000)
    
    // 2. 初始化 CTP 组件
    ctpClient := ctp.NewClient(rdb)
    ctpHandler := ctp.NewHandler(pg.DB, wsHub)
    
    // 3. 初始化策略执行器
    strategyExecutor := strategies.NewExecutor(pg.DB)
    
    // 4. 创建服务层
    tradingService := service.NewTradingService(pg.DB, ctpClient, ctpHandler, eventBus)
    marketService := service.NewMarketService(rdb, wsHub, ctpClient, eventBus)
    strategyService := service.NewStrategyService(pg.DB, strategyExecutor, tradingService, eventBus)
    
    return &Engine{
        cfg:             cfg,
        pg:              pg,
        rdb:             rdb,
        websocketHub:    wsHub,
        subs:            NewSubscriptionState(),
        stratExec:       strategyExecutor,
        ctpClient:       ctpClient,
        ctpHandler:      ctpHandler,
        eventBus:        eventBus,
        marketService:   marketService,
        tradingService:  tradingService,
        strategyService: strategyService,
    }
}
```

修改 `Start` 方法，注册事件处理器：

```go
func (e *Engine) Start(ctx context.Context) {
    log.Println("Starting Engine...")
    
    // 1. 注册事件处理器
    e.eventBus.Subscribe(constants.EventMarketDataReceived, e.marketService.HandleMarketData)
    e.eventBus.Subscribe("market.price.updated", e.strategyService.HandlePriceUpdate)
    e.eventBus.Subscribe("trade.response.received", e.tradingService.HandleTradeResponse)
    
    // 2. 加载策略
    e.stratExec.LoadActiveStrategies()
    
    // 3. 为活跃策略订阅行情
    for _, instID := range e.stratExec.GetSymbols() {
        log.Printf("Engine: Subscribing to %s for active strategies", instID)
        e.SubscribeSymbol(instID)
    }
    
    // 4. 启动 WebSocket Hub
    go e.websocketHub.Start()
    
    // 5. 启动行情监听
    e.marketService.StartMarketDataListener(ctx)
    
    // 6. 启动交易回报监听
    e.tradingService.StartTradeResponseListener(ctx)
    
    // 7. 启动查询回复订阅
    infra.StartQueryReplySubscriber(e.rdb, ctx)
    
    log.Println("Engine started.")
}
```

#### 阶段 2: 逐步迁移方法实现

将 Engine 的方法逐步委托给服务层：

```go
// 订阅合约 - 委托给 MarketService
func (e *Engine) SubscribeSymbol(instrumentID string) error {
    return e.marketService.Subscribe(instrumentID)
}

// 取消订阅 - 委托给 MarketService
func (e *Engine) UnsubscribeSymbol(instrumentID string) error {
    return e.marketService.Unsubscribe(instrumentID)
}

// 查询持仓 - 委托给 TradingService
func (e *Engine) QueryPositions(userID string, instrumentID string) error {
    return e.tradingService.QueryPositions(context.Background(), userID, instrumentID)
}

// 查询账户 - 委托给 TradingService
func (e *Engine) QueryAccount(userID string) error {
    return e.tradingService.QueryAccount(context.Background(), userID)
}
```

#### 阶段 3: 清理旧代码

完成迁移后：
1. 移除 Engine 中的旧实现
2. 移除不再使用的字段
3. 更新文档

## 🎯 架构改进效果

### 重构前
```
Engine (单体)
├── 行情订阅管理
├── 交易处理
├── 策略执行
├── WebSocket 管理
├── 数据库操作
└── CTP 通信
```

### 重构后
```
Engine (协调器)
├── EventBus (事件总线)
│   ├── 行情事件
│   ├── 交易事件
│   └── 策略事件
├── MarketService (行情服务)
│   ├── 订阅管理
│   └── 行情分发
├── TradingService (交易服务)
│   ├── 订单管理
│   └── 持仓更新
└── StrategyService (策略服务)
    ├── 策略加载
    └── 信号生成
```

## 💡 优势

1. **职责清晰** - 每个服务只负责一个领域
2. **易于测试** - 可以独立测试每个服务
3. **易于扩展** - 添加新功能只需创建新服务
4. **解耦合** - 通过事件总线通信，组件间无直接依赖
5. **可维护** - 代码组织更清晰，易于理解

## 📝 下一步行动

### 选项 1: 手动完成（推荐）

1. 手动编辑 `internal/ctp/client.go`，添加 `GetRedisClient` 方法
2. 运行 `go build ./...` 验证编译
3. 按照上面的步骤集成服务到 Engine
4. 逐步测试每个功能

### 选项 2: 暂时保留当前架构

1. 先使用新创建的服务层作为独立模块
2. 在新功能中使用新架构
3. 旧功能保持不变
4. 逐步迁移

### 选项 3: 回滚重构

如果觉得太复杂，可以：
1. 删除 `internal/service/`
2. 删除 `internal/event/`
3. 删除 `internal/constants/`
4. 恢复 `ctp/types.go` 中的常量
5. 恢复 `redis_pubsub.go` 的导入

## 🔍 建议

我强烈建议选择**选项 1**，因为：

1. 新架构确实更清晰、更易维护
2. 大部分工作已经完成
3. 只需要手动添加一个方法就能编译通过
4. 可以逐步迁移，不影响现有功能

重构是一个持续的过程，不需要一次性完成所有工作。先让代码能够编译运行，然后逐步优化。
