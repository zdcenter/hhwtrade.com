package model

import (
	"time"

	"gorm.io/gorm"
)

type OrderDirection string

const (
	DirectionBuy  OrderDirection = "0" // 买
	DirectionSell OrderDirection = "1" // 卖
)

// OrderOffset defines the open/close status.
type OrderOffset string

const (
	OffsetOpen           OrderOffset = "0" // 开仓
	OffsetClose          OrderOffset = "1" // 平仓
	OffsetCloseToday     OrderOffset = "3" // 平今 (上海期货交易所特有)
	OffsetCloseYesterday OrderOffset = "4" // 平昨 (上海期货交易所特有)
)

// OrderStatus defines the lifecycle status of an order.
type OrderStatus string

const (
	OrderStatusPending  OrderStatus = "0" // 待报送
	OrderStatusSent     OrderStatus = "1" // 已报送
	OrderStatusLocal    OrderStatus = "2" // 本地已受理 (CTP Accepted)
	OrderStatusFilled   OrderStatus = "3" // 全部成交
	OrderStatusCanceled OrderStatus = "4" // 已撤单
	OrderStatusRejected OrderStatus = "5" // 拒单/废单
	OrderStatusPartial  OrderStatus = "a" // 部分成交 (Custom code, not standard CTP status char but useful)
)

// Order represents a trade order record in the system.
// This is the "Master" record for a user's intent.
type Order struct {
	gorm.Model
	UserID       string         `gorm:"index;not null" json:"user_id"`
	Symbol       string         `gorm:"index;not null" json:"symbol"`
	Direction    OrderDirection `gorm:"type:varchar(1);not null" json:"direction"`
	Offset       OrderOffset    `gorm:"type:varchar(1);not null" json:"offset"`
	Price        float64        `gorm:"not null" json:"price"`
	Volume       int            `gorm:"not null" json:"volume"`
	FilledVolume int            `gorm:"default:0" json:"filled_volume"`
	Status       OrderStatus    `gorm:"type:varchar(1);index;default:'0'" json:"status"`
	
	// IDs
	OrderRef   string `gorm:"uniqueIndex" json:"order_ref"`        // Our local OrderRef
	OrderSysID string `gorm:"index" json:"order_sys_id"`           // Exchange Order ID
	StrategyID *uint  `gorm:"index" json:"strategy_id,omitempty"`  // Linked strategy

	// Error Info
	ErrorMsg string `json:"error_msg"`

	// Relations
	Trades []TradeRecord `gorm:"foreignKey:OrderID" json:"trades,omitempty"`
}

// TradeRecord represents a specific execution (deal) matched by the exchange.
// A single Order can have multiple TradeRecords (partial fills).
type TradeRecord struct {
	gorm.Model
	OrderID    uint    `gorm:"index;not null" json:"order_id"` // Foreign Key to Order
	TicketNo   string  `gorm:"index" json:"ticket_no"`         // Internal key (OrderRef) for redundancy
	
	TradeID    string  `gorm:"uniqueIndex" json:"trade_id"`    // Exchange Trade ID (Unique globally per exchange usually)
	Symbol     string  `gorm:"index" json:"symbol"`
	Direction  string  `json:"direction"`
	Offset     string  `json:"offset"`
	Price      float64 `json:"price"`
	Volume     int     `json:"volume"`
	TradeTime  string  `json:"trade_time"` // String from CTP, or parse to time.Time
}

// OrderStatusLog tracks the state transitions of an order.
// Useful for debugging why an order was rejected or how it executed over time.
type OrderStatusLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrderID   uint      `gorm:"index;not null" json:"order_id"`
	OldStatus string    `json:"old_status"`
	NewStatus string    `json:"new_status"`
	Message   string    `json:"message"` // e.g. "Order inserted", "Queueing", "All Traded"
	CreatedAt time.Time `json:"created_at"`
}
// Position represents a user's current holding in a specific contract.
type Position struct {
	UserID       string    `gorm:"primaryKey;index" json:"user_id"`
	Symbol       string    `gorm:"primaryKey;index" json:"symbol"`
	Direction    string    `gorm:"primaryKey" json:"direction"` // "long" or "short"
	TotalVolume  int       `json:"total_volume"`
	TodayVolume  int       `json:"today_volume"`
	AveragePrice float64   `json:"average_price"`
	UpdatedAt    time.Time `json:"updated_at"`
}
