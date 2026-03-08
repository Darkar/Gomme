package models

import "time"

type ScheduledTask struct {
	ID         uint       `gorm:"primarykey"`
	Name       string     `gorm:"not null"`
	PlaybookID uint       `gorm:"not null;index"`
	UserID     uint       `gorm:"not null;index"`
	CronExpr   string     `gorm:"not null"`
	Enabled    bool       `gorm:"default:true"`
	LastRunAt  *time.Time
	CreatedAt  time.Time
	Playbook   Playbook
	User       User
}
