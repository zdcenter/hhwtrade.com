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
	ID        uint           `gorm:"primaryKey" json:"ID"`
	UserID    string         `gorm:"index" json:"UserID"`
	Type      StrategyType   `json:"Type"`
	InstrumentID string      `gorm:"index" json:"InstrumentID"`
	Status    StrategyStatus `json:"Status"`
	Config    json.RawMessage `gorm:"type:jsonb" json:"Config"`
	CreatedAt time.Time      `json:"CreatedAt"`
	UpdatedAt time.Time      `json:"UpdatedAt"`
}

// ConditionOrderConfig defines the configuration structure for a basic condition order strategy.
type ConditionOrderConfig struct {
	TriggerPrice float64 `json:"TriggerPrice"`
	Operator     string  `json:"Operator"`
	Action       string  `json:"Action"`
	Volume       int     `json:"Volume"`
}
