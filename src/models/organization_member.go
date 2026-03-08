package models

type OrganizationMember struct {
	ID             uint `gorm:"primarykey"`
	OrganizationID uint `gorm:"not null;uniqueIndex:idx_org_user"`
	UserID         uint `gorm:"not null;uniqueIndex:idx_org_user"`

	// Playbooks
	CanCreatePlaybook bool `gorm:"default:false"`
	CanUpdatePlaybook bool `gorm:"default:false"`
	CanDeletePlaybook bool `gorm:"default:false"`

	// Inventaires
	CanCreateInventory bool `gorm:"default:false"`
	CanUpdateInventory bool `gorm:"default:false"`
	CanDeleteInventory bool `gorm:"default:false"`

	// Identifiants
	CanCreateCredential bool `gorm:"default:false"`
	CanUpdateCredential bool `gorm:"default:false"`
	CanDeleteCredential bool `gorm:"default:false"`

	// Repositories
	CanCreateRepository bool `gorm:"default:false"`
	CanUpdateRepository bool `gorm:"default:false"`
	CanDeleteRepository bool `gorm:"default:false"`

	Organization Organization `gorm:"foreignKey:OrganizationID"`
	User         User         `gorm:"foreignKey:UserID"`
}
