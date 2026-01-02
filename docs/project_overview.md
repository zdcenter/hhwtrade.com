# 项目概览（fiber_api）

本文档描述本项目的定位、核心模块职责，以及从「订阅合约 → CTP Core → Redis → Go 服务 → WebSocket/策略」的端到端流程。

---

## 1. 项目作用（你这个项目在做什么）

该项目是一个 **Go Fiber API 服务**，主要目标是把「交易/策略/行情」相关能力封装成：

- 对前端提供：
  - HTTP REST API（订阅管理、下单/撤单、策略管理等）
  - WebSocket 实时行情/事件推送
- 对后端 CTP Core（独立进程，常见是 Python）提供：
  - **通过 Redis** 发送指令（订阅行情、下单、撤单、查询）
  - **通过 Redis** 接收行情流和交易/查询回报
- 在服务内部提供：
  - 策略执行框架（Engine/StrategyService），消费行情触发策略下单

你的当前设计偏向「**单交易账号**」：

- CTP Core 侧登录/会话只对应一个交易账号
- 因此系统层面只有一份“全局订阅合约列表”是合理的
- 多个登录用户（Web UI 用户）共享同一份订阅列表，也共享同一份行情流

---

## 2. 模块划分与职责（按目录）

### 2.1 `cmd/main.go`

启动入口，完成依赖注入（DI）和后台协程启动。关键流程：

- 初始化 Postgres、Redis
- 创建 `WsManager`
- 创建 `ctp.Client`（发命令）和 `ctp.Handler`（处理回报）
- 创建 Service：
  - `MarketService`（订阅引用计数、触发 CTP 订阅/退订）
  - `TradingService`（下单/撤单/查询）
  - `SubscriptionService`（订阅列表落库、启动恢复订阅）
  - `StrategyService`（策略管理 + 行情驱动）
- 创建并启动 `Engine`
- 启动 `MarketDataDispatcher`（从全局行情通道分发到 WS 与 Engine）
- 启动 HTTP Server + 注册路由

### 2.2 `internal/api/*`

HTTP 与 WS 入口层。

- `router.go`：集中注册路由、Casbin 鉴权、依赖注入
- `ws_handler.go`：WebSocket 连接建立、接收前端 subscribe/unsubscribe 指令
- `subscription_handler.go`：订阅列表的 REST API
- `trade_handler.go`：下单/撤单/查询
- `strategy_handler.go`：策略相关

### 2.3 `internal/infra/*`

基础设施层（偏 IO 与并发）。

- `websocket.go`：`WsClient` + `WsManager`（连接管理、按 symbol 推送/全局广播）
- `redis_pubsub.go`（未在本文打开，但从现有 docs 可知）：订阅 Redis Pub/Sub，把行情写入 `MarketDataChan`
- `dispatcher.go`：`MarketDataDispatcher`，消费 `MarketDataChan` → 分发给 WS 与 Engine

### 2.4 `internal/ctp/*`

CTP 通信适配层（通过 Redis 与 CTP Core 通信）。

- `client.go`：把统一 Command 写入 Redis 队列（subscribe/unsubscribe/insert/cancel/query）
- `handler.go`：处理从 Redis 读到的交易/查询回报，更新数据库，并通过 notifier 推送事件

### 2.5 `internal/service/*`

业务服务层。

- `market_impl.go`：
  - 维护 `subscriptions map[string]int` 作为“订阅引用计数”
  - 首次订阅时才真正调用 `ctpClient.Subscribe`
  - 归零时才真正调用 `ctpClient.Unsubscribe`
- `subscription.go`：
  - 订阅列表落库/删除/排序/启动恢复
  - 当前实现**不含 UserID**，是全局订阅模型
- `trading_impl.go`：
  - 下单（生成 OrderRef → 发送 CTP → 异步写入 DB）
  - 撤单/查询

### 2.6 `internal/engine/engine.go`

协调器（轻量 engine）：

- 启动后台 Redis 订阅器（行情、查询回报、状态）
- 启动 WS Hub
- 消费交易回报队列（BRPOP）并调用 `ctpHandler.ProcessResponse`
- 提供 `OnMarketData` 给 `MarketDataDispatcher` 调用（策略入口）

---

## 3. 端到端流程（核心数据流）

下面按三条主链路描述：

### 3.1 行情链路（CTP Core → 前端 UI / 策略）

**链路：**

1. CTP Core（Python）从交易所收到行情
2. CTP Core 发布到 Redis（Pub/Sub channel：`ctp:market:*`）
3. Go 侧 `StartMarketDataSubscriber` 订阅 pattern，把消息写入全局 `infra.MarketDataChan`
4. `MarketDataDispatcher.Start()` 读取 `MarketDataChan`：
   - 调用 `wsManager.Broadcast(msg)` 推送给订阅该 symbol 的 WS 客户端
   - 调用 `engine.OnMarketData(msg)` 触发策略计算（可产生下单）

**简化后的结构图：**

```
CTP Core  ->  Redis PubSub  ->  StartMarketDataSubscriber  ->  MarketDataChan
                                                           |
                                                           |--> WsManager.Broadcast (UI)
                                                           |
                                                           '--> Engine.OnMarketData (Strategy)
```

### 3.2 订阅链路（前端 → Go → CTP Core）

订阅存在两种入口（按你现在代码）：

- **订阅列表（HTTP）**：`POST /api/subscriptions` → `SubscriptionService.AddSubscription` → `MarketService.Subscribe` → `ctpClient.Subscribe` → Redis 队列 → CTP Core
- **WS subscribe 消息**：`/ws` 收到 `{"Action":"subscribe"}` → `WsManager.Subscribe(client, instrumentID)`

注意：你当前 `ws_handler.go` 的 subscribe/unsubscribe 只影响 **WS 推送范围**（subscriptions map），并不会直接触发 CTP Core 订阅。

因此系统中的“订阅”实际上有两层：

- **系统层订阅（全局）**：由 `SubscriptionService` / `MarketService` 决定 CTP Core 到底订阅哪些合约
- **连接层订阅（每个 WS 连接）**：由 `WsManager` 决定该连接要不要接收某个 symbol 的推送

如果你希望“WS subscribe/unsubscribe 也能触发全局订阅”，需要在 `ws_handler.go` 引入 `MarketService` 并在收到 subscribe 时调用它（目前未做）。

### 3.3 交易与回报链路（前端 → Go → CTP Core → Go → 推送/落库）

1. 前端调用 HTTP 下单：`TradingService.PlaceOrder`
2. `ctpClient.InsertOrder` 把指令写入 Redis 队列
3. CTP Core 执行后把回报写入 Redis 队列（如 `constants.RedisQueueCTPResponse`）
4. `Engine.runTradeResponseLoop` 使用 `BRPOP` 消费回报
5. `ctpHandler.ProcessResponse`：
   - 更新数据库（订单状态、成交、持仓等）
   - 调用 notifier（当前是 `WsManager.PushToUser`，你已经退化成 Broadcast）推送给前端

---

## 4. 为什么会“才出现这个问题”（接口不匹配的根因）

你这次编译报错的根本原因不是 WsManager 复杂不复杂，而是 **项目里存在两套 Notifier 接口**：

- `internal/ctp/handler.go`：定义了 `ctp.Notifier`，只要求 `PushToUser(userID, data)`
- `internal/domain/interfaces.go`：定义了 `domain.Notifier`，要求 `BroadcastToAll/BroadcastMarketData`

当你在 `cmd/main.go` 把 `wsHub`（`*infra.WsManager`）注入到 `ctp.NewHandler(pg.DB, wsHub)` 时：

- 编译器只看 `ctp.Notifier`
- 而 `WsManager` 当时没有 `PushToUser`

所以编译失败。

这也解释了你感觉“之前取消按用户发送，WsManager 简化了，怎么反而有问题”：

- 你确实可以取消「按用户推送」的语义
- 但 **接口签名仍然需要存在**（哪怕实现退化为广播）

> 现状修复方式：在 `WsManager` 增加 `PushToUser`，内部转 `BroadcastToAll`。

---

## 5. “单账号 + 全局订阅”下，哪些可以简化（以及建议保留的点）

### 5.1 Subscription 是否需要 UserID？

你当前 `model.Subscription` 已经没有 `UserID` 字段，服务层也按全局订阅实现：

- `GetSubscriptions`：不按用户过滤
- `AddSubscription`：检查 `instrument_id` 是否存在（全局唯一）

这与“单交易账号、全局订阅”是一致的。

### 5.2 WsManager 可以简化到什么程度？

你现在的 `WsManager` 已经是简化版：

- 保留 `clients`（用于全局广播/系统消息/交易回报）
- 保留 `subscriptions`（按 symbol 推送行情给已 subscribe 的连接）
- 去掉了 `userConns` / `userID -> clients` 映射

在“所有登录用户看到同一份行情、同一份订阅列表”的情况下，进一步简化有两条路线：

- 路线 A（保留 symbol 订阅）：
  - 仍然让每个 WS 连接决定自己是否订阅某 symbol
  - 好处：前端可以控制自己屏幕要不要接收某 symbol，减少带宽
- 路线 B（彻底全局广播行情）：
  - `WsManager.Broadcast` 改成对所有连接广播（不再维护 `subscriptions` map）
  - 好处：结构最简单
  - 风险：前端连接越多/行情越多时，带宽与 CPU 开销线性增长，慢客户端更多

以你项目目前写法看，我更建议保留路线 A，因为你已经有 `subscribe/unsubscribe` 消息协议与 `subscriptions` 结构，成本不高，而且可以降低推送压力。

### 5.3 你现在代码里一个需要注意的点

`MarketServiceImpl.AddExistingSubscription` 里存在重复自增：

- 当前实现对同一个 instrument 做了 `++` 两次

这会让引用计数偏大，导致 “unsubscribe 不触发” 或 “恢复后计数异常”。

这不影响本文档目的，但如果你后面发现订阅计数不对，我建议优先修这里。

---

## 6. 文件索引（从流程角度）

- 启动/DI：`cmd/main.go`
- WS：`internal/api/ws_handler.go`, `internal/infra/websocket.go`
- 行情订阅 & 分发：`internal/infra/redis_pubsub.go`, `internal/infra/dispatcher.go`
- 策略入口：`internal/engine/engine.go`, `internal/service/strategy_impl.go`
- 交易请求：`internal/service/trading_impl.go`, `internal/ctp/client.go`
- 回报处理：`internal/engine/engine.go`（BRPOP）, `internal/ctp/handler.go`
- 订阅列表（全局）：`internal/model/subscription.go`, `internal/service/subscription.go`, `internal/api/subscription_handler.go`

---

## 7. 建议的下一步（如果你要继续“彻底简化”）

如果你确定未来不会做多交易账号/多租户：

- 把 `ctp.Notifier` 和 `domain.Notifier` 统一成一套接口（减少类似编译问题的机会）
- 明确订阅的“二层模型”（系统订阅 vs 连接订阅）是否都需要：
  - 如果只保留系统订阅：WS 永远广播所有活跃订阅合约
  - 如果保留连接订阅：WS subscribe/unsubscribe 只影响推送范围，不影响系统订阅

> 如果你告诉我你希望采用路线 A 还是路线 B，我可以进一步帮你把代码和 `docs/websocket_workflow.md` 同步到一致状态。
