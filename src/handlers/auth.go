package handlers

import (
	"gomme/models"
	"net/http"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

func (h *Handler) Index(c echo.Context) error {
	sess, _ := session.Get("gomme_session", c)
	if userID, ok := sess.Values["user_id"].(uint); ok && userID > 0 {
		return c.Redirect(http.StatusFound, "/dashboard")
	}
	return c.Redirect(http.StatusFound, "/login")
}

func (h *Handler) Login(c echo.Context) error {
	sess, _ := session.Get("gomme_session", c)
	if userID, ok := sess.Values["user_id"].(uint); ok && userID > 0 {
		return c.Redirect(http.StatusFound, "/dashboard")
	}
	return c.Render(http.StatusOK, "login", map[string]interface{}{
		"Error": "",
	})
}

func (h *Handler) LoginPost(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	var user models.User
	if err := h.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return c.Render(http.StatusOK, "login", map[string]interface{}{
			"Error": "Identifiant ou mot de passe incorrect",
		})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return c.Render(http.StatusOK, "login", map[string]interface{}{
			"Error": "Identifiant ou mot de passe incorrect",
		})
	}

	sess, _ := session.Get("gomme_session", c)
	sess.Values["user_id"] = user.ID
	sess.Save(c.Request(), c.Response())

	if user.MustChangePassword {
		return c.Redirect(http.StatusFound, "/change-password")
	}
	return c.Redirect(http.StatusFound, "/dashboard")
}

func (h *Handler) ChangePassword(c echo.Context) error {
	user := c.Get("user").(*models.User)
	return c.Render(http.StatusOK, "change_password", map[string]interface{}{
		"User":       user,
		"MustChange": user.MustChangePassword,
		"Error":      "",
	})
}

func (h *Handler) ChangePasswordPost(c echo.Context) error {
	user := c.Get("user").(*models.User)

	renderErr := func(msg string) error {
		return c.Render(http.StatusOK, "change_password", map[string]interface{}{
			"User":       user,
			"MustChange": user.MustChangePassword,
			"Error":      msg,
		})
	}

	// Vérifier le mot de passe actuel sauf lors d'un changement forcé
	if !user.MustChangePassword {
		current := c.FormValue("current_password")
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(current)); err != nil {
			return renderErr("Mot de passe actuel incorrect")
		}
	}

	newPw := c.FormValue("new_password")
	confirm := c.FormValue("confirm_password")

	if len(newPw) < 6 {
		return renderErr("Le mot de passe doit faire au moins 6 caractères")
	}
	if newPw != confirm {
		return renderErr("Les mots de passe ne correspondent pas")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPw), bcrypt.DefaultCost)
	if err != nil {
		return renderErr("Erreur interne")
	}

	h.DB.Model(user).Updates(map[string]interface{}{
		"password_hash":        string(hash),
		"must_change_password": false,
	})

	return c.Redirect(http.StatusFound, "/dashboard")
}

func (h *Handler) Logout(c echo.Context) error {
	sess, _ := session.Get("gomme_session", c)
	sess.Options.MaxAge = -1
	sess.Save(c.Request(), c.Response())
	return c.Redirect(http.StatusFound, "/login")
}
