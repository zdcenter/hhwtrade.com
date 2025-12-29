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

// BaseModel provides standard fields with PascalCase JSON tags for CTP/Frontend consistency
type BaseModel struct {
	ID        uint           `gorm:"primaryKey" json:"ID"`
	CreatedAt time.Time      `json:"CreatedAt"`
	UpdatedAt time.Time      `json:"UpdatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"DeletedAt,omitempty"`
}

// OrderOffset defines the open/close status (CombOffsetFlag in CTP)
type OrderOffset string

const (
	OffsetOpen           OrderOffset = "0" // 开仓
	OffsetClose          OrderOffset = "1" // 平仓
	OffsetCloseToday     OrderOffset = "3" // 平今
	OffsetCloseYesterday OrderOffset = "4" // 平昨
)

// OrderStatus defines the lifecycle status of an order (OrderStatus in CTP)
type OrderStatus string

const (
	OrderStatusAllTraded                OrderStatus = "0" // 全部成交
	OrderStatusPartTradedQueueing       OrderStatus = "1" // 部分成交还在队列中
	OrderStatusPartTradedNotQueueing    OrderStatus = "2" // 部分成交不在队列中
	OrderStatusNoTradeQueueing          OrderStatus = "3" // 未成交还在队列中
	OrderStatusNoTradeNotQueueing       OrderStatus = "4" // 未成交不在队列中
	OrderStatusCanceled                 OrderStatus = "5" // 撤单
	OrderStatusUnknown                  OrderStatus = "a" // 未知
	OrderStatusNotTouched               OrderStatus = "b" // 尚未触发
	OrderStatusTouched                  OrderStatus = "c" // 已触发
	OrderStatusPending                  OrderStatus = "P" // 内部状态: 待处理
	OrderStatusSent                     OrderStatus = "S" // 内部状态: 已发送
)

// Order aligns with CThostFtdcOrderField
type Order struct {
	BaseModel
	UserID       string `gorm:"index" json:"UserID"`
	InvestorID   string `json:"InvestorID"`
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

// Trade aligns with CThostFtdcTradeField
type Trade struct {
	BaseModel
	OrderID      uint    `gorm:"index" json:"OrderID"`
	OrderRef     string  `gorm:"index" json:"OrderRef"`
	OrderSysID   string  `gorm:"index" json:"OrderSysID"`
	TradeID      string  `gorm:"uniqueIndex" json:"TradeID"`
	InstrumentID string  `gorm:"index" json:"InstrumentID"`
	ExchangeID   string  `json:"ExchangeID"`
	Direction    string  `json:"Direction"`
	OffsetFlag   string  `json:"OffsetFlag"`
	Price        float64 `json:"Price"`
	Volume       int     `json:"Volume"`
	TradeDate    string  `json:"TradeDate"`
	TradeTime    string  `json:"TradeTime"`
	TradingDay   string  `json:"TradingDay"`
	StrategyID   *uint   `gorm:"index" json:"StrategyID,omitempty"`
}

type OrderLog struct {
	ID        uint      `gorm:"primaryKey" json:"ID"`
	OrderID   uint      `gorm:"index;not null" json:"OrderID"`
	OldStatus string    `json:"OldStatus"`
	NewStatus string    `json:"NewStatus"`
	Message   string    `json:"Message"`
	CreatedAt time.Time `json:"CreatedAt"`
}

// Position aligns with CThostFtdcInvestorPositionField critical fields
type Position struct {
	UserID       string `gorm:"primaryKey;index" json:"UserID"`
	InstrumentID string `gorm:"primaryKey;index" json:"InstrumentID"`

	// PosiDirection: '2'多, '3'空 (CTP: THOST_FTDC_PD_Long, THOST_FTDC_PD_Short)
	PosiDirection string `gorm:"primaryKey" json:"PosiDirection"`
	HedgeFlag     string `gorm:"primaryKey;default:'1'" json:"HedgeFlag"` // 投机/套保

	Position     int     `json:"Position"`       // 总持仓
	YdPosition   int     `json:"YdPosition"`    // 昨仓
	TodayPosition int     `json:"TodayPosition"` // 今仓
	
	PositionCost float64 `json:"PositionCost"` // 持仓成本
	AveragePrice float64 `json:"AveragePrice"` // 均价
	
	TradingDay   string    `json:"TradingDay"`
	UpdatedAt    time.Time `json:"UpdatedAt"`
}
