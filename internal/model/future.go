package model



// Future represents a tradable contract in the system.
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
	MarginRate           float64 `json:"MarginRate"`
}


