package constants

// 事件类型常量
const (
	// 行情事件
	EventMarketDataReceived = "market.data.received"
	EventMarketSubscribed   = "market.subscribed"
	EventMarketUnsubscribed = "market.unsubscribed"

	// 订单事件
	EventOrderPlaced   = "order.placed"
	EventOrderUpdated  = "order.updated"
	EventOrderFilled   = "order.filled"
	EventOrderCanceled = "order.canceled"
	EventOrderRejected = "order.rejected"

	// 成交事件
	EventTradeExecuted = "trade.executed"

	// 策略事件
	EventStrategyTriggered = "strategy.triggered"
	EventStrategyStarted   = "strategy.started"
	EventStrategyStopped   = "strategy.stopped"

	// 持仓事件
	EventPositionUpdated = "position.updated"
)
