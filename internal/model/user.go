package model

import "gorm.io/gorm"

// User represents a user in the system
type User struct {
	gorm.Model
	Username string `gorm:"uniqueIndex;not null" json:"username"`
	Email    string `gorm:"uniqueIndex;not null" json:"email"` // Mandatory and unique
	Password string `gorm:"not null" json:"-"`        // Stored as hash, ignored in JSON response
	Role     string `gorm:"default:'user'" json:"role"` // convenient field, though Casbin is the source of truth for permissions
	IsActive bool   `gorm:"default:true" json:"is_active"`
}
