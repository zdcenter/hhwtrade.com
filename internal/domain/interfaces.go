package domain

import (
	"context"

	"hhwtrade.com/internal/model"
)

// ===========================
// 订阅服务接口
// ===========================

// SubscriptionService 定义订阅相关的业务操作
type SubscriptionService interface {
	// 获取订阅列表
	GetSubscriptions(ctx context.Context, page, pageSize int) ([]model.Subscription, int64, error)
	// 添加订阅
	AddSubscription(ctx context.Context, instrumentID, exchangeID string) (*model.Subscription, error)
	// 移除订阅
	RemoveSubscription(ctx context.Context, instrumentID string) error
	// 重新排序订阅
	ReorderSubscriptions(ctx context.Context, instrumentIDs []string) error
	// 恢复所有已存储的订阅 (用于启动时)
	RestoreSubscriptions(ctx context.Context) error
}

// ===========================
// 行情服务接口
// ===========================

// MarketService 定义行情相关的业务操作
type MarketService interface {
	// 订阅合约行情 (发送到 CTP)
	Subscribe(ctx context.Context, instrumentID string) error
	// 取消订阅合约行情
	Unsubscribe(ctx context.Context, instrumentID string) error
	// 获取当前活跃订阅的合约
	GetActiveSymbols() []string
	// 同步合约信息
	SyncInstruments(ctx context.Context) error
	// 添加已存在的订阅 (用于恢复)
	AddExistingSubscription(instrumentID string)
	// 重新订阅所有活跃合约 (用于 CTP 重启恢复)
	ResubscribeAll(ctx context.Context) error
}

// ===========================
// 交易服务接口
// ===========================

// TradingService 定义交易相关的业务操作
type TradingService interface {
	// 下单
	PlaceOrder(ctx context.Context, order *model.Order) error
	// 撤单
	CancelOrder(ctx context.Context, orderID uint) error
	// 查询持仓 (触发 CTP 查询)
	QueryPositions(ctx context.Context, userID, instrumentID string) error
	// 查询账户 (触发 CTP 查询)
	QueryAccount(ctx context.Context, userID string) error
	// 获取订单列表
	GetOrders(ctx context.Context, userID string, page, pageSize int) ([]model.Order, int64, error)
	// 获取持仓列表
	GetPositions(ctx context.Context, userID string) ([]model.Position, error)
}

// ===========================
// 策略服务接口
// ===========================

// StrategyService 定义策略相关的业务操作
type StrategyService interface {
	// 创建策略
	CreateStrategy(ctx context.Context, strategy *model.Strategy) error
	// 停止策略
	StopStrategy(ctx context.Context, strategyID uint) error
	// 启动策略
	StartStrategy(ctx context.Context, strategyID uint) error
	// 获取用户策略列表
	GetStrategies(ctx context.Context, userID string, page, pageSize int) ([]model.Strategy, int64, error)
	// 获取策略详情
	GetStrategy(ctx context.Context, strategyID uint) (*model.Strategy, error)
	// 更新策略
	UpdateStrategy(ctx context.Context, strategyID uint, updates map[string]interface{}) error
	// 删除策略
	DeleteStrategy(ctx context.Context, strategyID uint) error
	// 获取活跃策略监控的合约列表
	GetActiveSymbols() []string
	// 重新加载策略
	Reload()
}

// ===========================
// WebSocket 推送接口
// ===========================

// Notifier 定义推送通知的接口
type Notifier interface {
	// 广播消息给所有连接的客户端 (用于系统通知/交易回报)
	BroadcastToAll(data interface{})
	// 广播行情数据
	BroadcastMarketData(data interface{})
}

// ===========================
// CTP 通信接口
// ===========================

// CTPClient 定义与 CTP 网关通信的接口
type CTPClienter interface {
	// 订阅行情
	Subscribe(ctx context.Context, instrumentID string) error
	// 取消订阅
	Unsubscribe(ctx context.Context, instrumentID string) error
	// 下单
	InsertOrder(ctx context.Context, order *model.Order) error
	// 撤单
	CancelOrder(ctx context.Context, order *model.Order) error
	// 查询持仓
	QueryPositions(ctx context.Context, userID, instrumentID string) error
	// 查询账户
	QueryAccount(ctx context.Context, userID string) error
	// 同步合约
	SyncInstruments(ctx context.Context) error
}

// ===========================
// 事件处理接口
// ===========================

// TradeResponseHandler 定义交易回报处理接口
type TradeResponseHandler interface {
	// 处理订单回报
	HandleOrderUpdate(ctx context.Context, orderRef string, status string, sysID string, msg string) error
	// 处理成交回报
	HandleTradeUpdate(ctx context.Context, orderRef string, price float64, volume int, tradeID string) error
	// 处理错误回报
	HandleOrderError(ctx context.Context, orderRef string, errorMsg string) error
	// 处理持仓查询结果
	HandlePositionQuery(ctx context.Context, positions []model.Position) error
	// 处理合约查询结果
	HandleInstrumentQuery(ctx context.Context, instruments []model.Future) error
}
