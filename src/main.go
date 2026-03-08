package main

import (
	"encoding/gob"
	"gomme/config"
	"gomme/db"
	"gomme/docker"
	"gomme/handlers"
	authmw "gomme/middleware"
	"gomme/models"
	"gomme/scheduler"
	"html/template"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/crypto/bcrypt"
)

func init() {
	gob.Register(uint(0))
}

type TemplateRenderer struct {
	templates *template.Template
}

func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func loadTemplates() (*template.Template, error) {
	root := template.New("").Funcs(template.FuncMap{
		"prev": func(n int) int { return n - 1 },
		"next": func(n int) int { return n + 1 },
		"splitOpts": func(opts string) [][2]string {
			var result [][2]string
			for _, line := range strings.Split(opts, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "|", 2)
				pair := [2]string{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[0])}
				if len(parts) == 2 {
					pair[1] = strings.TrimSpace(parts[1])
				}
				result = append(result, pair)
			}
			return result
		},
	})
	err := filepath.WalkDir("templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		name := strings.TrimPrefix(filepath.ToSlash(path), "templates/")
		name = strings.TrimSuffix(name, ".html")
		_, parseErr := root.New(name).Parse(string(content))
		return parseErr
	})
	return root, err
}

func main() {
	cfg := config.Load()

	database, err := db.Init(cfg.DSN)
	if err != nil {
		log.Fatalf("Erreur DB: %v", err)
	}

	var count int64
	database.Model(&models.User{}).Count(&count)
	if count == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		database.Create(&models.User{
			Username:           "admin",
			PasswordHash:       string(hash),
			IsAdmin:            true,
			MustChangePassword: true,
		})
		log.Println("Compte admin créé : admin / admin — changement de mot de passe requis à la première connexion")
	}

	tmpl, err := loadTemplates()
	if err != nil {
		log.Fatalf("Erreur templates: %v", err)
	}

	e := echo.New()
	e.HideBanner = true
	e.Renderer = &TemplateRenderer{templates: tmpl}
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(session.Middleware(sessions.NewCookieStore([]byte(cfg.SecretKey))))

	e.Static("/static", "static")

	dockerClient := docker.New(cfg.DockerHost)

	h := &handlers.Handler{DB: database, Config: cfg, Docker: dockerClient}

	sched := &scheduler.Scheduler{DB: database, RunFn: h.RunScheduledTask}
	sched.Start()

	e.GET("/", h.Index)
	e.GET("/login", h.Login)
	e.POST("/login", h.LoginPost)
	e.GET("/logout", h.Logout)

	auth := e.Group("", authmw.RequireAuth(database))

	auth.GET("/change-password", h.ChangePassword)
	auth.POST("/change-password", h.ChangePasswordPost)

	auth.GET("/profile", h.Profile)
	auth.POST("/profile/password", h.ProfilePasswordPost)

	auth.GET("/dashboard", h.Dashboard)
	auth.GET("/history", h.History)

	auth.GET("/inventory", h.InventoryRedirect)
	auth.GET("/inventory/list", h.InventoryList)
	auth.POST("/inventory", h.InventoryCreate)
	auth.GET("/inventory/:id", h.InventoryDetail)
	auth.POST("/inventory/:id/edit", h.InventoryUpdate)
	auth.POST("/inventory/:id/delete", h.InventoryDelete)
	auth.POST("/inventory/:id/sync", h.InventorySync)
	auth.POST("/inventory/:id/groups", h.InventoryGroupCreate)
	auth.POST("/inventory/:id/groups/:gid/delete", h.InventoryGroupDelete)
	auth.POST("/inventory/:id/hosts", h.InventoryHostCreate)
	auth.POST("/inventory/:id/hosts/:hid/edit", h.InventoryHostUpdate)
	auth.POST("/inventory/:id/hosts/:hid/delete", h.InventoryHostDelete)
	auth.GET("/api/inventory/:id/hosts/:hid", h.InventoryHostAPI)
	auth.POST("/inventory/:id/vars", h.InventoryVarsSave)
	auth.GET("/api/inventory/:id/vars", h.InventoryVarsAPI)
	auth.GET("/inventory/:id/graph", h.InventoryGraphDetail)
	auth.GET("/api/inventory/:id/graph", h.InventoryGraphDetailAPI)

	auth.GET("/repository", h.RepositoryList)
	auth.POST("/repository", h.RepositoryCreate)
	auth.POST("/repository/:id/edit", h.RepositoryUpdate)
	auth.POST("/repository/:id/delete", h.RepositoryDelete)
	auth.POST("/repository/:id/sync", h.RepositorySync)

	auth.GET("/playbooks", h.PlaybookList)
	auth.POST("/playbooks", h.PlaybookCreate)
	auth.GET("/playbooks/:id", h.PlaybookDetail)
	auth.GET("/playbooks/:id/edit", h.PlaybookEdit)
	auth.POST("/playbooks/:id/edit", h.PlaybookUpdate)
	auth.POST("/playbooks/:id/delete", h.PlaybookDelete)
	auth.POST("/playbooks/:id/run", h.PlaybookRun)
	auth.POST("/playbooks/:id/org", h.PlaybookUpdateOrg)
	auth.GET("/api/playbooks/:id/survey", h.PlaybookSurveyAPI)
	auth.GET("/api/playbooks/:id/inventories", h.PlaybookInventoriesAPI)
	auth.POST("/playbooks/:id/inventories", h.PlaybookInventoryAdd)
	auth.POST("/playbooks/:id/inventories/:lid/delete", h.PlaybookInventoryDelete)
	auth.GET("/playbooks/:id/vars", h.PlaybookVarsList)
	auth.POST("/playbooks/:id/vars", h.PlaybookVarsSave)
	auth.GET("/playbooks/:id/survey", h.PlaybookSurvey)
	auth.POST("/playbooks/:id/survey", h.PlaybookSurveySave)
	auth.POST("/playbooks/:id/credentials", h.PlaybookCredentialsSave)
	auth.GET("/runs/:id", h.RunDetail)
	auth.GET("/api/runs/:id/output", h.RunOutput)
	auth.GET("/api/runs/:id/stream", h.RunLogsStream)
	auth.GET("/api/repository/:id/files", h.RepositoryFiles)

	auth.GET("/tasks", h.TaskList)
	auth.POST("/tasks", h.TaskCreate)
	auth.POST("/tasks/:id/delete", h.TaskDelete)
	auth.POST("/tasks/:id/toggle", h.TaskToggle)

	auth.GET("/credentials", h.CredentialList)
	auth.POST("/credentials", h.CredentialCreate)
	auth.POST("/credentials/:id/edit", h.CredentialUpdate)
	auth.POST("/credentials/:id/delete", h.CredentialDelete)
	auth.GET("/api/credentials/:id/fields", h.CredentialFieldsAPI)

	auth.GET("/organizations", h.OrganizationList)
	auth.POST("/organizations", h.OrganizationCreate)
	auth.GET("/organizations/:id", h.OrganizationDetail)
	auth.POST("/organizations/:id/edit", h.OrganizationUpdate)
	auth.POST("/organizations/:id/delete", h.OrganizationDelete)
	auth.POST("/organizations/:id/transfer", h.OrganizationTransfer)
	auth.POST("/organizations/:id/members", h.OrganizationMemberAdd)
	auth.POST("/organizations/:id/members/:uid/update", h.OrganizationMemberUpdate)
	auth.POST("/organizations/:id/members/:uid/delete", h.OrganizationMemberRemove)

	admin := auth.Group("/admin", authmw.RequireAdmin)
	admin.GET("", h.AdminRedirect)
	admin.GET("/users", h.AdminUsers)
	admin.POST("/users", h.AdminUserCreate)
	admin.POST("/users/:id/edit", h.AdminUserUpdate)
	admin.POST("/users/:id/delete", h.AdminUserDelete)
	admin.GET("/playbooks", h.AdminPlaybooks)
	admin.GET("/settings", h.AdminSettings)
	admin.POST("/settings", h.AdminSettingsSave)
	admin.GET("/images", h.AdminImages)
	admin.POST("/images", h.AdminImageCreate)
	admin.POST("/images/:id/delete", h.AdminImageDelete)

	log.Printf("Démarrage sur le port %s", cfg.Port)
	log.Fatal(e.Start(":" + cfg.Port))
}
