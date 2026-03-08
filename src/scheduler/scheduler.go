package scheduler

import (
	"gomme/models"
	"log"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Scheduler struct {
	DB    *gorm.DB
	RunFn func(taskID uint)
}

func (s *Scheduler) Start() {
	go func() {
		for {
			now := time.Now()
			next := now.Truncate(time.Minute).Add(time.Minute)
			time.Sleep(time.Until(next))
			s.tick(time.Now())
		}
	}()
	log.Println("Scheduler démarré")
}

func (s *Scheduler) tick(t time.Time) {
	var tasks []models.ScheduledTask
	s.DB.Where("enabled = ?", true).Find(&tasks)
	for _, task := range tasks {
		if cronMatches(task.CronExpr, t) {
			log.Printf("Scheduler : déclenchement de la tâche %d (%s)", task.ID, task.Name)
			id := task.ID
			go s.RunFn(id)
		}
	}
}

// cronMatches vérifie si t correspond à une expression cron à 5 champs (min heure dom mois dow).
func cronMatches(expr string, t time.Time) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false
	}
	return matchField(fields[0], t.Minute(), 0, 59) &&
		matchField(fields[1], t.Hour(), 0, 23) &&
		matchField(fields[2], t.Day(), 1, 31) &&
		matchField(fields[3], int(t.Month()), 1, 12) &&
		matchField(fields[4], int(t.Weekday()), 0, 6)
}

func matchField(field string, val, _, _ int) bool {
	if field == "*" {
		return true
	}
	for _, part := range strings.Split(field, ",") {
		if strings.HasPrefix(part, "*/") {
			n, err := strconv.Atoi(part[2:])
			if err == nil && n > 0 && val%n == 0 {
				return true
			}
		} else if idx := strings.Index(part, "-"); idx != -1 {
			lo, err1 := strconv.Atoi(part[:idx])
			rest := part[idx+1:]
			step := 1
			if si := strings.Index(rest, "/"); si != -1 {
				step, _ = strconv.Atoi(rest[si+1:])
				rest = rest[:si]
			}
			hi, err2 := strconv.Atoi(rest)
			if err1 == nil && err2 == nil && val >= lo && val <= hi {
				if step <= 1 || (val-lo)%step == 0 {
					return true
				}
			}
		} else {
			n, err := strconv.Atoi(part)
			if err == nil && n == val {
				return true
			}
		}
	}
	return false
}
