package handlers

import (
	"gomme/config"
	"gomme/docker"

	"gorm.io/gorm"
)

type Handler struct {
	DB     *gorm.DB
	Config *config.Config
	Docker *docker.Client
}
