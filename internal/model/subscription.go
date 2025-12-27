package model

import (
	"time"
)

// UserSubscription stores the user's favorite symbols.
type UserSubscription struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       string    `gorm:"index;uniqueIndex:idx_user_inst" json:"user_id"`
	InstrumentID string    `gorm:"uniqueIndex:idx_user_inst" json:"instrument_id"` // 改为 InstrumentID
	ExchangeID   string    `json:"exchange_id"`                                    // 改为 ExchangeID
	Sorter       int       `json:"sorter"`
	CreatedAt    time.Time `json:"created_at"`
}

// FuturesContract represents a tradable contract in the system.
type FuturesContract struct {
	InstrumentID         string  `gorm:"primaryKey" json:"instrument_id"`    // 改为 InstrumentID
	ExchangeID           string  `json:"exchange_id"`                        // CTP: ExchangeID
	InstrumentName       string  `gorm:"index" json:"instrument_name"`       // CTP: InstrumentName
	ProductID            string  `gorm:"index" json:"product_id"`            // CTP: ProductID
	PriceTick            float64 `json:"price_tick"`                         // CTP: PriceTick
	VolumeMultiple       int     `json:"volume_multiple"`                    // CTP: VolumeMultiple
	MaxMarketOrderVolume int     `json:"max_market_order_volume"`            // CTP: MaxMarketOrderVolume
	MinMarketOrderVolume int     `json:"min_market_order_volume"`            // CTP: MinMarketOrderVolume
	MaxLimitOrderVolume  int     `json:"max_limit_order_volume"`             // CTP: MaxLimitOrderVolume
	MinLimitOrderVolume  int     `json:"min_limit_order_volume"`             // CTP: MinLimitOrderVolume
	ExpireDate           string  `json:"expire_date"`                        // CTP: ExpireDate
	IsTrading            int     `json:"is_trading"`                         // CTP: IsTrading
}

func (FuturesContract) TableName() string {
	return "futures_contracts"
}
