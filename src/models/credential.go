package models

// Credential est un identifiant global (non lié à un playbook spécifique).
// Il peut appartenir à une organisation ou à un utilisateur à titre personnel.
// Ses champs (CredentialField) sont injectés comme variables Ansible (--extra-vars).
type Credential struct {
	ID             uint               `gorm:"primarykey"`
	Name           string             `gorm:"not null"`
	OrganizationID *uint              `gorm:"index"`
	UserID         uint               `gorm:"not null;index"`
	Organization   *Organization      `gorm:"foreignKey:OrganizationID"`
	User           User               `gorm:"foreignKey:UserID"`
	Fields         []CredentialField  `gorm:"foreignKey:CredentialID"`
}
