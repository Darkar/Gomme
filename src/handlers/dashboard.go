package handlers

import (
	"fmt"
	"gomme/models"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type DashboardData struct {
	User         *models.User
	TotalRuns    int64
	SuccessRuns  int64
	FailedRuns   int64
	RunningRuns  int64
	RecentRuns   []models.PlaybookRun
	ChartLabels  string
	ChartSuccess string
	ChartFailed  string
}

func (h *Handler) Dashboard(c echo.Context) error {
	user := c.Get("user").(*models.User)
	data := DashboardData{User: user}

	h.DB.Model(&models.PlaybookRun{}).Where("user_id = ?", user.ID).Count(&data.TotalRuns)
	h.DB.Model(&models.PlaybookRun{}).Where("user_id = ? AND status = ?", user.ID, "success").Count(&data.SuccessRuns)
	h.DB.Model(&models.PlaybookRun{}).Where("user_id = ? AND status = ?", user.ID, "failed").Count(&data.FailedRuns)
	h.DB.Model(&models.PlaybookRun{}).Where("user_id = ? AND status = ?", user.ID, "running").Count(&data.RunningRuns)

	h.DB.Preload("Playbook").Preload("Inventories.Inventory").
		Where("user_id = ?", user.ID).
		Order("id desc").
		Limit(10).
		Find(&data.RecentRuns)

	data.ChartLabels, data.ChartSuccess, data.ChartFailed = h.buildChartData(user.ID)

	return c.Render(http.StatusOK, "dashboard", data)
}

func (h *Handler) buildChartData(userID uint) (labels, successes, failures string) {
	type dayCount struct {
		Day     string
		Success int
		Failed  int
	}

	var results []dayCount
	start := time.Now().AddDate(0, 0, -29)
	h.DB.Raw(`
		SELECT DATE(started_at) as day,
		       CAST(SUM(status = 'success') AS UNSIGNED) as success,
		       CAST(SUM(status = 'failed') AS UNSIGNED) as failed
		FROM playbook_runs
		WHERE user_id = ? AND started_at >= ?
		GROUP BY DATE(started_at)
		ORDER BY day ASC
	`, userID, start).Scan(&results)

	// Indexer par date pour accès O(1)
	byDay := make(map[string]dayCount, len(results))
	for _, r := range results {
		byDay[r.Day] = r
	}

	// Générer les 30 jours consécutifs, 0 pour les jours sans données
	lbls := make([]string, 30)
	scs := make([]string, 30)
	fls := make([]string, 30)
	for i := 0; i < 30; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		lbls[i] = fmt.Sprintf("%q", day)
		if r, ok := byDay[day]; ok {
			scs[i] = fmt.Sprintf("%d", r.Success)
			fls[i] = fmt.Sprintf("%d", r.Failed)
		} else {
			scs[i] = "0"
			fls[i] = "0"
		}
	}
	return strings.Join(lbls, ","), strings.Join(scs, ","), strings.Join(fls, ",")
}
