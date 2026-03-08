package handlers

import (
	"fmt"
	"gomme/models"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type TopPlaybook struct {
	PlaybookID uint
	Name       string
	Total      int64
	Success    int64
	Rate       int // pourcentage arrondi
}

type DashboardData struct {
	User         *models.User
	TotalRuns    int64
	SuccessRuns  int64
	FailedRuns   int64
	RunningRuns  int64
	SuccessRate  int
	AvgDuration  string
	ActiveRuns   []models.PlaybookRun
	TopPlaybooks []TopPlaybook
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

	if data.TotalRuns > 0 {
		data.SuccessRate = int(data.SuccessRuns * 100 / data.TotalRuns)
	}

	// Durée moyenne des runs terminés
	data.AvgDuration = h.buildAvgDuration(user.ID)

	// Runs en cours
	h.DB.Preload("Playbook").Preload("Inventories.Inventory").
		Where("user_id = ? AND status = ?", user.ID, "running").
		Order("id desc").
		Find(&data.ActiveRuns)

	// Top 5 playbooks
	data.TopPlaybooks = h.buildTopPlaybooks(user.ID)

	h.DB.Preload("Playbook").Preload("Inventories.Inventory").
		Where("user_id = ?", user.ID).
		Order("id desc").
		Limit(10).
		Find(&data.RecentRuns)

	data.ChartLabels, data.ChartSuccess, data.ChartFailed = h.buildChartData(user.ID)

	return c.Render(http.StatusOK, "dashboard", data)
}

func (h *Handler) buildAvgDuration(userID uint) string {
	var avgSeconds float64
	h.DB.Raw(`
		SELECT COALESCE(AVG(TIMESTAMPDIFF(SECOND, started_at, finished_at)), 0)
		FROM playbook_runs
		WHERE user_id = ? AND status IN ('success', 'failed')
		  AND started_at IS NOT NULL AND finished_at IS NOT NULL
	`, userID).Scan(&avgSeconds)

	if avgSeconds == 0 {
		return "-"
	}
	total := int(avgSeconds)
	if total < 60 {
		return fmt.Sprintf("%ds", total)
	}
	return fmt.Sprintf("%dm%ds", total/60, total%60)
}

func (h *Handler) buildTopPlaybooks(userID uint) []TopPlaybook {
	type row struct {
		PlaybookID uint
		Name       string
		Total      int64
		Success    int64
	}
	var rows []row
	h.DB.Raw(`
		SELECT pr.playbook_id, p.name, COUNT(*) as total,
		       CAST(SUM(pr.status = 'success') AS UNSIGNED) as success
		FROM playbook_runs pr
		JOIN playbooks p ON p.id = pr.playbook_id
		WHERE pr.user_id = ?
		GROUP BY pr.playbook_id, p.name
		ORDER BY total DESC
		LIMIT 5
	`, userID).Scan(&rows)

	result := make([]TopPlaybook, 0, len(rows))
	for _, r := range rows {
		rate := 0
		if r.Total > 0 {
			rate = int(r.Success * 100 / r.Total)
		}
		result = append(result, TopPlaybook{
			PlaybookID: r.PlaybookID,
			Name:       r.Name,
			Total:      r.Total,
			Success:    r.Success,
			Rate:       rate,
		})
	}
	return result
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

	byDay := make(map[string]dayCount, len(results))
	for _, r := range results {
		byDay[r.Day] = r
	}

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
