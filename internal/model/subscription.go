package model

import (
	"time"
)

// Subscription stores the user's favorite symbols.
type Subscription struct {
	ID           uint      `gorm:"primaryKey" json:"ID"`
	UserID       string    `gorm:"index;uniqueIndex:idx_user_inst" json:"UserID"`
	InstrumentID string    `gorm:"uniqueIndex:idx_user_inst" json:"InstrumentID"`
	ExchangeID   string    `json:"ExchangeID"`
	Sorter       int       `json:"Sorter"`
	CreatedAt    time.Time `json:"CreatedAt"`
}

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
}


