package models

import "time"

type Inventory struct {
	ID             uint          `gorm:"primarykey"`
	Name           string        `gorm:"not null"`
	Source         string        `gorm:"not null"` // manual, proxmox, ad, ocs, vcenter
	Config         string        `gorm:"type:text"`
	LastSyncAt     *time.Time
	OrganizationID *uint         // nil = propriété de l'utilisateur
	UserID         uint          `gorm:"index"`
	Organization   *Organization `gorm:"foreignKey:OrganizationID"`
	Hosts          []Host        `gorm:"foreignKey:InventoryID"`
	Groups         []Group       `gorm:"foreignKey:InventoryID"`
}

type Host struct {
	ID          uint    `gorm:"primarykey"`
	InventoryID uint    `gorm:"not null;index"`
	Name        string  `gorm:"not null"`
	IP          string
	Description string
	Vars        string  `gorm:"type:text"`
	Groups      []Group `gorm:"many2many:host_groups;"`
}

type Group struct {
	ID          uint   `gorm:"primarykey"`
	InventoryID uint   `gorm:"not null;index"`
	Name        string `gorm:"not null"`
	Description string
	Hosts       []Host `gorm:"many2many:host_groups;"`
}

// InventoryVar représente une variable Ansible appliquée à tous les hôtes de l'inventaire ([all:vars]).
type InventoryVar struct {
	ID          uint   `gorm:"primarykey"`
	InventoryID uint   `gorm:"not null;index"`
	Key         string `gorm:"not null"`
	Value       string `gorm:"type:text"`
}
