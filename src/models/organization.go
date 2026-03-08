package models

import "time"

type Organization struct {
	ID        uint      `gorm:"primarykey"`
	Name      string    `gorm:"uniqueIndex;not null"`
	OwnerID   uint      `gorm:"not null;index"`
	Owner     User      `gorm:"foreignKey:OwnerID"`
	CreatedAt time.Time
	Members   []OrganizationMember `gorm:"foreignKey:OrganizationID"`
}
