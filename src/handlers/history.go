package handlers

import (
	"gomme/models"
	"math"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

const historyPerPage = 25

type HistoryData struct {
	User       *models.User
	Runs       []models.PlaybookRun
	Total      int64
	Page       int
	TotalPages int
	Query      string
	Status     string
	Pages      []int // numéros de pages à afficher
}

func (h *Handler) History(c echo.Context) error {
	user := c.Get("user").(*models.User)

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	query := c.QueryParam("q")
	statusFilter := c.QueryParam("status")

	// Identifiants des organisations dont l'utilisateur est membre
	var orgIDs []uint
	h.DB.Model(&models.OrganizationMember{}).
		Where("user_id = ?", user.ID).
		Pluck("organization_id", &orgIDs)

	// Base query : runs créés par l'utilisateur OU appartenant à ses orgs
	db := h.DB.Model(&models.PlaybookRun{}).
		Joins("JOIN playbooks ON playbooks.id = playbook_runs.playbook_id")

	if user.IsAdmin {
		// L'admin voit tout
	} else if len(orgIDs) > 0 {
		db = db.Where("playbook_runs.user_id = ? OR playbooks.organization_id IN ?", user.ID, orgIDs)
	} else {
		db = db.Where("playbook_runs.user_id = ?", user.ID)
	}

	// Filtre par statut
	if statusFilter != "" {
		db = db.Where("playbook_runs.status = ?", statusFilter)
	}

	// Filtre par nom de playbook
	if query != "" {
		db = db.Where("playbooks.name LIKE ?", "%"+query+"%")
	}

	var total int64
	db.Count(&total)

	totalPages := int(math.Ceil(float64(total) / float64(historyPerPage)))
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	var runs []models.PlaybookRun
	db.Preload("Playbook").Preload("Inventories.Inventory").Preload("User").
		Order("playbook_runs.id DESC").
		Limit(historyPerPage).
		Offset((page - 1) * historyPerPage).
		Find(&runs)

	data := HistoryData{
		User:       user,
		Runs:       runs,
		Total:      total,
		Page:       page,
		TotalPages: totalPages,
		Query:      query,
		Status:     statusFilter,
		Pages:      buildPageRange(page, totalPages),
	}

	return c.Render(http.StatusOK, "history", data)
}

// buildPageRange retourne les numéros de pages à afficher autour de la page courante.
func buildPageRange(current, total int) []int {
	const window = 2
	pages := []int{}
	seen := map[int]bool{}

	add := func(p int) {
		if p >= 1 && p <= total && !seen[p] {
			seen[p] = true
			pages = append(pages, p)
		}
	}

	add(1)
	for i := current - window; i <= current+window; i++ {
		add(i)
	}
	add(total)

	// Trier
	for i := 0; i < len(pages)-1; i++ {
		for j := i + 1; j < len(pages); j++ {
			if pages[i] > pages[j] {
				pages[i], pages[j] = pages[j], pages[i]
			}
		}
	}
	return pages
}
