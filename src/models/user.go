package models

import "time"

type User struct {
	ID                 uint      `gorm:"primarykey"`
	Username           string    `gorm:"uniqueIndex;not null"`
	PasswordHash       string    `gorm:"not null"`
	IsAdmin            bool      `gorm:"default:false"`
	MustChangePassword bool      `gorm:"default:false"`
	CreatedAt          time.Time
}
