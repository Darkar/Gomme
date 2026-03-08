package handlers

import (
	"gomme/docker"
	"gomme/models"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type AdminUsersData struct {
	User    *models.User
	Users   []models.User
	Success string
	Error   string
}

type AdminPlaybooksData struct {
	User *models.User
	Runs []models.PlaybookRun
}

type AdminSettingsData struct {
	User     *models.User
	Settings map[string]string
	Success  string
}

func (h *Handler) AdminRedirect(c echo.Context) error {
	return c.Redirect(http.StatusFound, "/admin/users")
}

func (h *Handler) AdminUsers(c echo.Context) error {
	user := c.Get("user").(*models.User)
	data := AdminUsersData{
		User:    user,
		Success: c.QueryParam("success"),
		Error:   c.QueryParam("error"),
	}
	h.DB.Find(&data.Users)
	return c.Render(http.StatusOK, "admin/users", data)
}

func (h *Handler) AdminUserCreate(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")
	isAdmin := c.FormValue("is_admin") == "on"

	if username == "" || password == "" {
		return c.Redirect(http.StatusFound, "/admin/users?error=Champs+requis")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return c.Redirect(http.StatusFound, "/admin/users?error=Erreur+interne")
	}

	user := models.User{
		Username:     username,
		PasswordHash: string(hash),
		IsAdmin:      isAdmin,
	}
	if err := h.DB.Create(&user).Error; err != nil {
		return c.Redirect(http.StatusFound, "/admin/users?error=Identifiant+déjà+utilisé")
	}
	return c.Redirect(http.StatusFound, "/admin/users?success=Utilisateur+créé")
}

func (h *Handler) AdminUserUpdate(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var target models.User
	if err := h.DB.First(&target, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/admin/users?error=Utilisateur+introuvable")
	}

	if pw := c.FormValue("password"); pw != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			return c.Redirect(http.StatusFound, "/admin/users?error=Erreur+interne")
		}
		target.PasswordHash = string(hash)
	}
	target.IsAdmin = c.FormValue("is_admin") == "on"
	h.DB.Save(&target)
	return c.Redirect(http.StatusFound, "/admin/users?success=Utilisateur+mis+à+jour")
}

func (h *Handler) AdminUserDelete(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	current := c.Get("user").(*models.User)
	if current.ID == uint(id) {
		return c.Redirect(http.StatusFound, "/admin/users?error=Impossible+de+supprimer+son+propre+compte")
	}
	h.DB.Delete(&models.User{}, id)
	return c.Redirect(http.StatusFound, "/admin/users?success=Utilisateur+supprimé")
}

func (h *Handler) AdminPlaybooks(c echo.Context) error {
	user := c.Get("user").(*models.User)
	data := AdminPlaybooksData{User: user}
	h.DB.Preload("Playbook").Preload("User").Preload("Inventories.Inventory").
		Order("id desc").Limit(100).Find(&data.Runs)
	return c.Render(http.StatusOK, "admin/playbooks", data)
}

func (h *Handler) AdminSettings(c echo.Context) error {
	user := c.Get("user").(*models.User)
	data := AdminSettingsData{
		User:     user,
		Settings: map[string]string{},
		Success:  c.QueryParam("success"),
	}
	var settings []models.Setting
	h.DB.Find(&settings)
	for _, s := range settings {
		data.Settings[s.Key] = s.Value
	}
	return c.Render(http.StatusOK, "admin/settings", data)
}

func (h *Handler) AdminSettingsSave(c echo.Context) error {
	keys := []string{"sync_interval"}
	for _, key := range keys {
		val := c.FormValue(key)
		var setting models.Setting
		result := h.DB.Where("key = ?", key).First(&setting)
		if result.Error != nil {
			h.DB.Create(&models.Setting{Key: key, Value: val})
		} else {
			h.DB.Model(&setting).Update("value", val)
		}
	}
	return c.Redirect(http.StatusFound, "/admin/settings?success=Paramètres+sauvegardés")
}

type AdminImagesData struct {
	User         *models.User
	Images       []models.ExecutionImage
	DockerImages []docker.Image
	Success      string
	Error        string
}

func (h *Handler) AdminImages(c echo.Context) error {
	user := c.Get("user").(*models.User)
	data := AdminImagesData{
		User:    user,
		Success: c.QueryParam("success"),
		Error:   c.QueryParam("error"),
	}
	h.DB.Order("name").Find(&data.Images)
	if imgs, err := h.Docker.ListImages(); err == nil {
		data.DockerImages = imgs
	}
	return c.Render(http.StatusOK, "admin/images", data)
}

func (h *Handler) AdminImageCreate(c echo.Context) error {
	name := c.FormValue("name")
	if name == "" {
		return c.Redirect(http.StatusFound, "/admin/images?error=Nom+requis")
	}
	img := models.ExecutionImage{
		Name:        name,
		Description: c.FormValue("description"),
	}
	if err := h.DB.Create(&img).Error; err != nil {
		return c.Redirect(http.StatusFound, "/admin/images?error=Image+déjà+existante")
	}
	return c.Redirect(http.StatusFound, "/admin/images?success=Image+ajoutée")
}

func (h *Handler) AdminImageDelete(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	h.DB.Delete(&models.ExecutionImage{}, id)
	return c.Redirect(http.StatusFound, "/admin/images?success=Image+supprimée")
}
