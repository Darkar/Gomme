package db

import (
	"gomme/models"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	maxRetries = 20
	retryDelay = 3 * time.Second
)

func Init(dsn string) (*gorm.DB, error) {
	var database *gorm.DB
	var err error

	for i := 1; i <= maxRetries; i++ {
		database, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Warn),
		})
		if err == nil {
			sqlDB, pingErr := database.DB()
			if pingErr == nil {
				pingErr = sqlDB.Ping()
			}
			if pingErr == nil {
				break
			}
			err = pingErr
		}
		log.Printf("DB non disponible (%d/%d) : %v — nouvelle tentative dans %s", i, maxRetries, err, retryDelay)
		time.Sleep(retryDelay)
	}

	if err != nil {
		return nil, err
	}

	err = database.AutoMigrate(
		&models.User{},
		&models.Organization{},
		&models.OrganizationMember{},
		&models.Inventory{},
		&models.Host{},
		&models.Group{},
		&models.Repository{},
		&models.Playbook{},
		&models.PlaybookInventoryLink{},
		&models.PlaybookRun{},
		&models.PlaybookRunInventory{},
		&models.InventoryVar{},
		&models.PlaybookVar{},
		&models.SurveyField{},
		&models.Credential{},
		&models.CredentialField{},
		&models.Setting{},
		&models.ExecutionImage{},
		&models.ScheduledTask{},
	)
	return database, err
}
