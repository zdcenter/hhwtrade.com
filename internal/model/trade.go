package model

import (
	"time"
)

// OrderDirection defines the trade direction.
type OrderDirection string

const (
	DirectionBuy  OrderDirection = "buy"  // 买
	DirectionSell OrderDirection = "sell" // 卖
)

// OrderOffset defines the open/close status.
type OrderOffset string

const (
	OffsetOpen       OrderOffset = "open"        // 开仓
	OffsetClose      OrderOffset = "close"       // 平仓
	OffsetCloseToday OrderOffset = "close_today" // 平今 (上海期货交易所特有)
)

// OrderStatus defines the lifecycle status of an order.
type OrderStatus string

const (
	OrderStatusPending  OrderStatus = "pending"  // 待报送
	OrderStatusSent     OrderStatus = "sent"     // 已报送
	OrderStatusLocal    OrderStatus = "local"    // 本地已受理
	OrderStatusFilled   OrderStatus = "filled"   // 全部成交
	OrderStatusCanceled OrderStatus = "canceled" // 已撤单
	OrderStatusRejected OrderStatus = "rejected" // 拒单/废单
)

// Order represents a trade order record in the system.
type Order struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID string `gorm:"index" json:"user_id"`

	// Request Info
	Symbol    string         `gorm:"index" json:"symbol"`
	Direction OrderDirection `json:"direction"`  // buy, sell
	Offset    OrderOffset    `json:"offset"`     // open, close
	PriceType string         `json:"price_type"` // limit, market (default limit)
	Price     float64        `json:"price"`      // Limit price
	Volume    int            `json:"volume"`

	// Status Info
	Status    OrderStatus `gorm:"index" json:"status"`
	RequestID string      `gorm:"uniqueIndex" json:"request_id"` // Unique ID for matching with CTP response

	// Exchange Info
	OrderSysID   string `gorm:"index" json:"order_sys_id"` // ID given by Exchange (Required for cancellation)
	FilledVolume int    `json:"filled_volume"`             // Accumulated filled volume

	// Source Info
	StrategyID *uint `gorm:"index" json:"strategy_id"` // null for manual orders

	// Error Info
	ErrorMsg string `json:"error_msg"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Position represents a user's holding position.
// This is the "Go-side" logic position, synchronized from CTP core.
type Position struct {
	UserID    string `gorm:"primaryKey" json:"user_id"`
	Symbol    string `gorm:"primaryKey" json:"symbol"`
	Direction string `gorm:"primaryKey" json:"direction"` // "long" or "short" (CTP通常分多空两个持仓Record)

	TotalVolume  int     `json:"total_volume"`  // 总持仓
	TodayVolume  int     `json:"today_volume"`  // 今仓 (可平今)
	FrozenVolume int     `json:"frozen_volume"` // 挂单冻结数量
	AveragePrice float64 `json:"average_price"` // 持仓均价

	UpdatedAt time.Time `json:"updated_at"`
}
