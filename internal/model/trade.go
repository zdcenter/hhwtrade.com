package model

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel 为 CTP/前端一致性提供带有 PascalCase JSON 标签的标准字段
type BaseModel struct {
	ID        uint           `gorm:"primaryKey" json:"ID"`
	CreatedAt time.Time      `json:"CreatedAt"`
	UpdatedAt time.Time      `json:"UpdatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"DeletedAt,omitempty"`
}

// Trade 与 CThostFtdcTradeField 对齐
type Trade struct {
	BaseModel
	OrderID      uint    `gorm:"index" json:"OrderID"`
	InvestorID   string  `gorm:"index" json:"InvestorID"`
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
