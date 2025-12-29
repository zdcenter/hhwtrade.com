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

