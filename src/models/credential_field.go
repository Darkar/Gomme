package models

// CredentialField est un champ d'un identifiant.
// Secret=true : chiffré en base, masqué dans l'UI et dans les logs d'exécution.
type CredentialField struct {
	ID           uint   `gorm:"primarykey"`
	CredentialID uint   `gorm:"not null;index"`
	Key          string `gorm:"not null"`
	ValueEnc     string `gorm:"type:text"` // toujours chiffré
	Secret       bool   `gorm:"default:false"`
}
