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
	// CTP 标准状态 (TThostFtdcOrderStatusType)
	OrderStatusAllTraded                OrderStatus = "0" // 全部成交
	OrderStatusPartTradedQueueing       OrderStatus = "1" // 部分成交还在队列中
	OrderStatusPartTradedNotQueueing    OrderStatus = "2" // 部分成交不在队列中
	OrderStatusNoTradeQueueing          OrderStatus = "3" // 未成交还在队列中
	OrderStatusNoTradeNotQueueing       OrderStatus = "4" // 未成交不在队列中
	OrderStatusCanceled                 OrderStatus = "5" // 撤单
	OrderStatusUnknown                  OrderStatus = "a" // 未知
	OrderStatusNotTouched               OrderStatus = "b" // 尚未触发
	OrderStatusTouched                  OrderStatus = "c" // 已触发

	// 内部自定义中间状态 (Internal States)
	OrderStatusPending OrderStatus = "P" // 待入库 (Pending)
	OrderStatusSent    OrderStatus = "S" // 已发送至 CTP (Sent)
)

// Order represents a trade order record in the system.
type Order struct {
	gorm.Model
	UserID       string         `gorm:"index;not null" json:"user_id"`
	InstrumentID string         `gorm:"index;not null" json:"instrument_id"` // 改为 InstrumentID
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
type TradeRecord struct {
	gorm.Model
	OrderID      uint    `gorm:"index;not null" json:"order_id"`
	TicketNo     string  `gorm:"index" json:"ticket_no"`         // OrderRef
	TradeID      string  `gorm:"uniqueIndex" json:"trade_id"`
	InstrumentID string  `gorm:"index" json:"instrument_id"`      // 改为 InstrumentID
	Direction    string  `json:"direction"`
	Offset       string  `json:"offset"`
	Price        float64 `json:"price"`
	Volume       int     `json:"volume"`
	TradeTime    string  `json:"trade_time"`
}

// Position represents a user's current holding.
type Position struct {
	UserID       string    `gorm:"primaryKey;index" json:"user_id"`
	InstrumentID string    `gorm:"primaryKey;index" json:"instrument_id"` // 改为 InstrumentID
	Direction    string    `gorm:"primaryKey" json:"direction"`           // "long" or "short"
	TotalVolume  int       `json:"total_volume"`
	TodayVolume  int       `json:"today_volume"`
	AveragePrice float64   `json:"average_price"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type OrderStatusLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrderID   uint      `gorm:"index;not null" json:"order_id"`
	OldStatus string    `json:"old_status"`
	NewStatus string    `json:"new_status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
