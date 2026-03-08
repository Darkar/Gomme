package models

import "time"

type Playbook struct {
	ID           uint          `gorm:"primarykey"`
	RepositoryID uint          `gorm:"not null;index"`
	Path         string        `gorm:"not null"`
	Name         string        `gorm:"not null"`
	Description  string
	OrganizationID *uint         // nil = propriété de l'utilisateur
	UserID         uint          `gorm:"index"`
	DockerImage    string
	DefaultLimit   string
	Organization   *Organization `gorm:"foreignKey:OrganizationID"`
	Repository     Repository
	Inventories    []PlaybookInventoryLink `gorm:"foreignKey:PlaybookID"`
	Vars           []PlaybookVar           `gorm:"foreignKey:PlaybookID"`
	SurveyFields   []SurveyField           `gorm:"foreignKey:PlaybookID"`
	Credentials    []Credential            `gorm:"many2many:playbook_credential_links;"`
}

type PlaybookRun struct {
	ID          uint       `gorm:"primarykey"`
	PlaybookID  uint       `gorm:"not null;index"`
	UserID      uint       `gorm:"not null;index"`
	Limit       string
	DockerImage string
	ContainerID string
	Status      string     `gorm:"default:pending"` // pending, running, success, failed
	Output      string     `gorm:"type:longtext"`
	StartedAt   *time.Time
	FinishedAt  *time.Time
	Playbook    Playbook
	User        User
	Inventories []PlaybookRunInventory `gorm:"foreignKey:RunID"`
}

// PlaybookInventoryLink — inventaires associés à un playbook (pré-sélectionnés au lancement)
type PlaybookInventoryLink struct {
	ID          uint      `gorm:"primarykey"`
	PlaybookID  uint      `gorm:"not null;index"`
	InventoryID uint      `gorm:"not null"`
	GroupFilter string    // groupe optionnel → filtre le contenu INI généré
	Inventory   Inventory `gorm:"foreignKey:InventoryID"`
}

// PlaybookRunInventory — inventaires réellement utilisés lors d'un run
type PlaybookRunInventory struct {
	ID          uint      `gorm:"primarykey"`
	RunID       uint      `gorm:"not null;index"`
	InventoryID uint      `gorm:"not null"`
	GroupFilter string
	Inventory   Inventory `gorm:"foreignKey:InventoryID"`
}

type PlaybookVar struct {
	ID         uint   `gorm:"primarykey"`
	PlaybookID uint   `gorm:"not null;index"`
	Key        string `gorm:"not null"`
	Value      string `gorm:"type:text"`
	Encrypted  bool   `gorm:"default:false"`
}

type SurveyField struct {
	ID         uint   `gorm:"primarykey"`
	PlaybookID uint   `gorm:"not null;index"`
	Label      string `gorm:"not null"`
	VarName    string `gorm:"not null"`
	Type       string `gorm:"default:text"` // text, textarea, select, bool
	Options    string `gorm:"type:text"`
	Default    string `gorm:"type:text"`
	Required   bool   `gorm:"default:false"`
	SortOrder  int
}

type Setting struct {
	ID    uint   `gorm:"primarykey"`
	Key   string `gorm:"uniqueIndex;not null"`
	Value string `gorm:"type:text"`
}

// ExecutionImage représente une image Docker approuvée par un admin pour exécuter des playbooks.
type ExecutionImage struct {
	ID          uint   `gorm:"primarykey"`
	Name        string `gorm:"not null;uniqueIndex"` // ex: "cytopia/ansible:latest"
	Description string
}
