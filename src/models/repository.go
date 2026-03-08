package models

import "time"

type Repository struct {
	ID        uint       `gorm:"primarykey"`
	Name      string     `gorm:"not null"`
	URL       string     `gorm:"not null"`
	Branch    string     `gorm:"default:main"`
	LocalPath string
	AutoSync  bool       `gorm:"default:false"`
	LastSyncAt *time.Time

	OrganizationID *uint         `gorm:"index"`
	UserID         uint          `gorm:"index"`
	Organization   *Organization `gorm:"foreignKey:OrganizationID"`

	// Authentification
	AuthType    string `gorm:"default:none"`      // "none", "password", "ssh_key"
	Username    string
	PasswordEnc string `gorm:"type:text"`         // mot de passe ou token chiffré (AES-GCM)
	SSHKeyEnc   string `gorm:"type:text"`         // clé privée SSH chiffrée (AES-GCM)
	InsecureTLS bool   `gorm:"default:false"`     // GIT_SSL_NO_VERIFY

	Playbooks []Playbook `gorm:"foreignKey:RepositoryID"`
}
