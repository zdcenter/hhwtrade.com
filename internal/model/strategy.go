package model

import (
	"encoding/json"
	"time"
)

// StrategyType defines the supported strategy types.
type StrategyType string

const (
	StrategyTypeConditionOrder StrategyType = "condition_order"
	StrategyTypeGridTrading    StrategyType = "grid_trading"
)

// StrategyStatus defines the lifecycle status of a strategy.
type StrategyStatus string

const (
	StrategyStatusActive    StrategyStatus = "active"
	StrategyStatusStopped   StrategyStatus = "stopped"
	StrategyStatusCompleted StrategyStatus = "completed"
	StrategyStatusError     StrategyStatus = "error"
)

// Strategy represents a user's running strategy instance.
type Strategy struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID string `gorm:"index" json:"user_id"`

	// Strategy Logic Type
	Type StrategyType `json:"type"`

	// Target Symbol
	Symbol string `gorm:"index" json:"symbol"`

	// Lifecycle Status
	Status StrategyStatus `json:"status"`

	// Dynamic Configuration (JSON)
	// Example for ConditionOrder: {"trigger_price": 3000, "operator": ">", "action": "open_long", "volume": 1}
	Config json.RawMessage `gorm:"type:jsonb" json:"config"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConditionOrderConfig defines the configuration structure for a basic condition order strategy.
type ConditionOrderConfig struct {
	TriggerPrice float64 `json:"trigger_price"` // Trigger Price
	Operator     string  `json:"operator"`      // ">", ">=", "<", "<="
	Action       string  `json:"action"`        // "open_long", "close_long", "open_short", "close_short"
	Volume       int     `json:"volume"`        // Volume to trade
}
