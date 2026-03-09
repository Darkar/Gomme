package middleware

import (
	"gomme/i18n"
	"gomme/models"

	"github.com/labstack/echo/v4"
)

// LangMiddleware detects the user's preferred language and sets it in the context.
// Priority: user preference (Language field) → Accept-Language header → "en".
func LangMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			lang := ""
			if user, ok := c.Get("user").(*models.User); ok && user != nil && user.Language != "" {
				lang = user.Language
			}
			if lang == "" {
				lang = i18n.Detect(c.Request().Header.Get("Accept-Language"))
			}
			c.Set("lang", lang)
			return next(c)
		}
	}
}
