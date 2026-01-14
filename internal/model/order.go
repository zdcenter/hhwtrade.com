package model

import (
	"time"
)

type OrderDirection string

const (
	DirectionBuy  OrderDirection = "0" // 买
	DirectionSell OrderDirection = "1" // 卖
)

// OrderOffset 定义开平仓状态（CTP 中的 CombOffsetFlag）
type OrderOffset string

const (
	OffsetOpen           OrderOffset = "0" // 开仓
	OffsetClose          OrderOffset = "1" // 平仓
	OffsetCloseToday     OrderOffset = "3" // 平今
	OffsetCloseYesterday OrderOffset = "4" // 平昨
)

// OrderStatus 定义订单的生命周期状态（CTP 中的 OrderStatus）
type OrderStatus string

const (
	OrderStatusAllTraded             OrderStatus = "0" // 全部成交
	OrderStatusPartTradedQueueing    OrderStatus = "1" // 部分成交还在队列中
	OrderStatusPartTradedNotQueueing OrderStatus = "2" // 部分成交不在队列中
	OrderStatusNoTradeQueueing       OrderStatus = "3" // 未成交还在队列中
	OrderStatusNoTradeNotQueueing    OrderStatus = "4" // 未成交不在队列中
	OrderStatusCanceled              OrderStatus = "5" // 撤单
	OrderStatusUnknown               OrderStatus = "a" // 未知
	OrderStatusNotTouched            OrderStatus = "b" // 尚未触发
	OrderStatusTouched               OrderStatus = "c" // 已触发
	OrderStatusPending               OrderStatus = "P" // 内部状态: 待处理
	OrderStatusSent                  OrderStatus = "S" // 内部状态: 已发送
)

// Order 与 CThostFtdcOrderField 对齐
type Order struct {
	BaseModel
	InvestorID   string `gorm:"index" json:"InvestorID"`
	InstrumentID string `gorm:"index" json:"InstrumentID"`
	ExchangeID   string `json:"ExchangeID"`
	OrderRef     string `gorm:"uniqueIndex" json:"OrderRef"`

	Direction      OrderDirection `gorm:"type:varchar(1)" json:"Direction"`
	CombOffsetFlag OrderOffset    `gorm:"type:varchar(1)" json:"CombOffsetFlag"`

	LimitPrice          float64 `json:"LimitPrice"`
	VolumeTotalOriginal int     `json:"VolumeTotalOriginal"`
	VolumeTraded        int     `gorm:"default:0" json:"VolumeTraded"`

	OrderStatus OrderStatus `gorm:"type:varchar(1);index" json:"OrderStatus"`
	OrderSysID  string      `gorm:"index" json:"OrderSysID"`
	StatusMsg   string      `json:"StatusMsg"`

	FrontID   int `json:"FrontID"`
	SessionID int `json:"SessionID"`

	TradingDay string `json:"TradingDay"`
	InsertDate string `json:"InsertDate"`
	InsertTime string `json:"InsertTime"`

	StrategyID *uint   `gorm:"index" json:"StrategyID,omitempty"`
	Trades     []Trade `gorm:"foreignKey:OrderID" json:"Trades,omitempty"`
}

type OrderLog struct {
	ID        uint      `gorm:"primaryKey" json:"ID"`
	OrderID   uint      `gorm:"index;not null" json:"OrderID"`
	OldStatus string    `json:"OldStatus"`
	NewStatus string    `json:"NewStatus"`
	Message   string    `json:"Message"`
	CreatedAt time.Time `json:"CreatedAt"`
}
