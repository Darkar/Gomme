package handlers

import (
	"fmt"
	"gomme/models"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type TaskListData struct {
	User     *models.User
	Tasks    []models.ScheduledTask
	Playbooks []models.Playbook
	Success  string
	Error    string
}

func (h *Handler) TaskList(c echo.Context) error {
	user := c.Get("user").(*models.User)
	data := TaskListData{
		User:    user,
		Success: c.QueryParam("success"),
		Error:   c.QueryParam("error"),
	}
	h.DB.Preload("Playbook").Preload("User").Order("created_at desc").Find(&data.Tasks)
	h.DB.Find(&data.Playbooks)
	return c.Render(http.StatusOK, "tasks/list", data)
}

func (h *Handler) TaskCreate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	name := strings.TrimSpace(c.FormValue("name"))
	cronExpr := strings.TrimSpace(c.FormValue("cron_expr"))
	pbIDStr := c.FormValue("playbook_id")
	source := c.FormValue("source") // "playbook" → redirect to edit page

	if name == "" || cronExpr == "" || pbIDStr == "" {
		redirect := "/tasks?error=Nom%2C+expression+cron+et+playbook+requis"
		if source == "playbook" {
			redirect = fmt.Sprintf("/playbooks/%s/edit?tab=schedule&error=Nom%%2C+expression+cron+et+playbook+requis", pbIDStr)
		}
		return c.Redirect(http.StatusFound, redirect)
	}

	if len(strings.Fields(cronExpr)) != 5 {
		redirect := "/tasks?error=Expression+cron+invalide+%285+champs+requis%29"
		if source == "playbook" {
			redirect = fmt.Sprintf("/playbooks/%s/edit?tab=schedule&error=Expression+cron+invalide", pbIDStr)
		}
		return c.Redirect(http.StatusFound, redirect)
	}

	pbID, _ := strconv.ParseUint(pbIDStr, 10, 64)
	task := models.ScheduledTask{
		Name:       name,
		PlaybookID: uint(pbID),
		UserID:     user.ID,
		CronExpr:   cronExpr,
		Enabled:    true,
	}
	h.DB.Create(&task)

	if source == "playbook" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?tab=schedule&success=Tâche+créée", pbID))
	}
	return c.Redirect(http.StatusFound, "/tasks?success=Tâche+créée")
}

func (h *Handler) TaskDelete(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	source := c.FormValue("source")
	pbIDStr := c.FormValue("playbook_id")
	h.DB.Delete(&models.ScheduledTask{}, id)
	if source == "playbook" && pbIDStr != "" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%s/edit?tab=schedule&success=Tâche+supprimée", pbIDStr))
	}
	return c.Redirect(http.StatusFound, "/tasks?success=Tâche+supprimée")
}

func (h *Handler) TaskToggle(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	source := c.FormValue("source")
	pbIDStr := c.FormValue("playbook_id")
	var task models.ScheduledTask
	if err := h.DB.First(&task, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/tasks")
	}
	h.DB.Model(&task).Update("enabled", !task.Enabled)
	if source == "playbook" && pbIDStr != "" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%s/edit?tab=schedule", pbIDStr))
	}
	return c.Redirect(http.StatusFound, "/tasks")
}

// RunScheduledTask est appelé par le scheduler pour exécuter une tâche planifiée.
func (h *Handler) RunScheduledTask(taskID uint) {
	var task models.ScheduledTask
	if err := h.DB.Preload("Playbook.Repository").
		Preload("Playbook.Vars").
		Preload("Playbook.SurveyFields").
		Preload("Playbook.Credentials.Fields").
		Preload("Playbook.Inventories").
		First(&task, taskID).Error; err != nil {
		return
	}

	pb := task.Playbook
	if pb.DockerImage == "" {
		return
	}

	var runInvs []RunInventory
	for _, link := range pb.Inventories {
		runInvs = append(runInvs, RunInventory{
			InventoryID: link.InventoryID,
			GroupFilter: link.GroupFilter,
		})
	}

	now := time.Now()
	run := models.PlaybookRun{
		PlaybookID:  pb.ID,
		UserID:      task.UserID,
		Limit:       pb.DefaultLimit,
		DockerImage: pb.DockerImage,
		Status:      "running",
		StartedAt:   &now,
	}
	if err := h.DB.Create(&run).Error; err != nil {
		return
	}
	for _, ri := range runInvs {
		h.DB.Create(&models.PlaybookRunInventory{
			RunID:       run.ID,
			InventoryID: ri.InventoryID,
			GroupFilter: ri.GroupFilter,
		})
	}

	runID := run.ID
	h.DB.Model(&task).Update("last_run_at", &now)

	go func() {
		output, _, err := h.executePlaybook(&pb, runInvs, pb.DefaultLimit, pb.DockerImage, map[string]string{}, runID)
		status := "success"
		if err != nil {
			status = "failed"
			output += "\n\nErreur: " + err.Error()
		}
		finished := time.Now()
		h.DB.Model(&models.PlaybookRun{ID: runID}).Updates(map[string]interface{}{
			"status":      status,
			"output":      output,
			"finished_at": &finished,
		})
	}()
}
