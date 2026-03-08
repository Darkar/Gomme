package middleware

import (
	"gomme/models"
	"net/http"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func RequireAuth(db *gorm.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sess, _ := session.Get("gomme_session", c)
			userID, ok := sess.Values["user_id"].(uint)
			if !ok || userID == 0 {
				return c.Redirect(http.StatusFound, "/login")
			}
			var user models.User
			if err := db.First(&user, userID).Error; err != nil {
				return c.Redirect(http.StatusFound, "/login")
			}
			c.Set("user", &user)
			if user.MustChangePassword && c.Path() != "/change-password" {
				return c.Redirect(http.StatusFound, "/change-password")
			}
			return next(c)
		}
	}
}

func RequireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, ok := c.Get("user").(*models.User)
		if !ok || !user.IsAdmin {
			return echo.ErrForbidden
		}
		return next(c)
	}
}
