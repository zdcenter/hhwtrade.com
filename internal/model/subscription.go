package model

import (
	"time"
)

// UserSubscription stores the user's favorite symbols.
type UserSubscription struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"index;uniqueIndex:idx_user_symbol" json:"user_id"` // User ID, indexed and unique with Symbol
	Symbol    string    `gorm:"uniqueIndex:idx_user_symbol" json:"symbol"`        // Symbol code, e.g., "rb2601"
	Exchange  string    `json:"exchange"`                                         // Exchange, e.g., "SHFE"
	Sorter    int       `json:"sorter"`                                           // Sorter
	CreatedAt time.Time `json:"created_at"`
}

// Instrument represents a tradable contract/symbol in the system.
// This can be used for searching symbols.
type Instrument struct {
	Symbol   string `gorm:"primaryKey" json:"symbol"` // e.g., "rb2601"
	Name     string `json:"name"`                     // e.g., "Steel Rebar 2601"
	Exchange string `json:"exchange"`                 // e.g., "SHFE"
	Type     string `json:"type"`                     // e.g., "future", "stock"
}
