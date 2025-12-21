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

// FuturesContract represents a tradable contract/symbol in the system.
// This can be used for searching symbols.
type FuturesContract struct {
	Symbol           string `gorm:"primaryKey" json:"symbol"`           // e.g., "rb2601"
	Name             string `gorm:"index" json:"name"`                  // e.g., "Steel Rebar 2601"
	Exchange         string `json:"exchange"`                           // e.g., "SHFE"
	ProductID        string `gorm:"index" json:"product_id"`            // e.g., "rb" (Product code)
	UnderlyingSymbol string `json:"underlying_symbol"`                  // For options/derivatives
	Type             string `json:"type"`                               // e.g., "future", "options"
	ExpiryDate       string `json:"expiry_date"`                        // Optional: "20260115"
}

// TableName explicitly sets the database table name for the FuturesContract model.
func (FuturesContract) TableName() string {
	return "futures_contracts"
}
