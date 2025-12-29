package model


// User represents a user in the system
type User struct {
	BaseModel
	Username string `gorm:"uniqueIndex;not null" json:"Username"`
	Email    string `gorm:"uniqueIndex;not null" json:"Email"`
	Password string `gorm:"not null" json:"-"`
	Role     string `gorm:"default:'user'" json:"Role"`
	IsActive bool   `gorm:"default:true" json:"IsActive"`
}
