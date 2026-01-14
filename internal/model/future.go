package model

// Future 表示系统中的可交易合约
type Future struct {
	InstrumentID         string  `gorm:"primaryKey" json:"InstrumentID"`
	ExchangeID           string  `json:"ExchangeID"`
	InstrumentName       string  `gorm:"index" json:"InstrumentName"`
	ProductID            string  `gorm:"index" json:"ProductID"`
	PriceTick            float64 `json:"PriceTick"`
	VolumeMultiple       int     `json:"VolumeMultiple"`
	MaxMarketOrderVolume int     `json:"MaxMarketOrderVolume"`
	MinMarketOrderVolume int     `json:"MinMarketOrderVolume"`
	MaxLimitOrderVolume  int     `json:"MaxLimitOrderVolume"`
	MinLimitOrderVolume  int     `json:"MinLimitOrderVolume"`
	ExpireDate           string  `json:"ExpireDate"`
	IsTrading            int     `json:"IsTrading"`
	IsActive             bool    `gorm:"default:true" json:"IsActive"`
	MarginRate           float64 `json:"MarginRate"` // 此时该字段可作为参考或废弃

	// 详细保证金率
	LongMarginRatioByMoney   float64 `json:"LongMarginRatioByMoney"`   // 多头保证金率(按金额)
	LongMarginRatioByVolume  float64 `json:"LongMarginRatioByVolume"`  // 多头保证金率(按手数)
	ShortMarginRatioByMoney  float64 `json:"ShortMarginRatioByMoney"`  // 空头保证金率(按金额)
	ShortMarginRatioByVolume float64 `json:"ShortMarginRatioByVolume"` // 空头保证金率(按手数)

	// 详细手续费率
	OpenRatioByMoney        float64 `json:"OpenRatioByMoney"`        // 开仓手续费率(按金额)
	OpenRatioByVolume       float64 `json:"OpenRatioByVolume"`       // 开仓手续费率(按手数)
	CloseRatioByMoney       float64 `json:"CloseRatioByMoney"`       // 平仓手续费率(按金额)
	CloseRatioByVolume      float64 `json:"CloseRatioByVolume"`      // 平仓手续费率(按手数)
	CloseTodayRatioByMoney  float64 `json:"CloseTodayRatioByMoney"`  // 平今手续费率(按金额)
	CloseTodayRatioByVolume float64 `json:"CloseTodayRatioByVolume"` // 平今手续费率(按手数)
}
