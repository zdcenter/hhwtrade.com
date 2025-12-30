package model

import (
	"encoding/json"
	"time"
)

// StrategyType 定义支持的策略类型
type StrategyType string

const (
	StrategyTypeConditionOrder StrategyType = "condition_order"
	StrategyTypeGridTrading    StrategyType = "grid_trading"
)

// StrategyStatus 定义策略的生命周期状态
type StrategyStatus string

const (
	StrategyStatusActive    StrategyStatus = "active"
	StrategyStatusStopped   StrategyStatus = "stopped"
	StrategyStatusCompleted StrategyStatus = "completed"
	StrategyStatusError     StrategyStatus = "error"
)

// Strategy 表示用户正在运行的策略实例
type Strategy struct {
	ID           uint            `gorm:"primaryKey" json:"ID"`
	UserID       string          `gorm:"index" json:"UserID"`
	Type         StrategyType    `json:"Type"`
	InstrumentID string          `gorm:"index" json:"InstrumentID"`
	Status       StrategyStatus  `json:"Status"`
	Config       json.RawMessage `gorm:"type:jsonb" json:"Config"`
	CreatedAt    time.Time       `json:"CreatedAt"`
	UpdatedAt    time.Time       `json:"UpdatedAt"`
}

// ConditionOrderConfig 定义基本条件单策略的配置结构
type ConditionOrderConfig struct {
	TriggerPrice float64 `json:"TriggerPrice"`
	Operator     string  `json:"Operator"`
	Action       string  `json:"Action"`
	Volume       int     `json:"Volume"`
}
