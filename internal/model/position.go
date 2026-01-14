package model

import (
	"time"
)

// Position 与 CThostFtdcInvestorPositionField 关键字段对齐
type Position struct {
	InvestorID   string `gorm:"primaryKey;index" json:"InvestorID"`
	InstrumentID string `gorm:"primaryKey;index" json:"InstrumentID"`
	ExchangeID   string `json:"ExchangeID"`

	// PosiDirection: '2'多, '3'空 (CTP: THOST_FTDC_PD_Long, THOST_FTDC_PD_Short)
	PosiDirection string `gorm:"primaryKey" json:"PosiDirection"`
	HedgeFlag     string `gorm:"primaryKey;default:'1'" json:"HedgeFlag"` // 投机/套保

	Position      int `json:"Position"`      // 总持仓
	YdPosition    int `json:"YdPosition"`    // 昨仓
	TodayPosition int `json:"TodayPosition"` // 今仓

	PositionCost   float64 `json:"PositionCost"`   // 持仓成本
	OpenCost       float64 `json:"OpenCost"`       // 开仓成本
	UseMargin      float64 `json:"UseMargin"`      // 占用保证金
	FrozenMargin   float64 `json:"FrozenMargin"`   // 冻结保证金
	PositionProfit float64 `json:"PositionProfit"` // 持仓盈亏
	CloseProfit    float64 `json:"CloseProfit"`    // 平仓盈亏
	Commission     float64 `json:"Commission"`     // 手续费
	AveragePrice   float64 `json:"AveragePrice"`   // 均价 (计算得出)

	TradingDay string    `json:"TradingDay"`
	UpdatedAt  time.Time `json:"UpdatedAt"`
}
