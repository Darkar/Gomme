package handlers

import (
	"gomme/models"
	"net/http"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type ProfileData struct {
	User    *models.User
	Success string
	Error   string
}

func (h *Handler) Profile(c echo.Context) error {
	user := c.Get("user").(*models.User)
	return c.Render(http.StatusOK, "profile", ProfileData{
		User:    user,
		Success: c.QueryParam("success"),
		Error:   c.QueryParam("error"),
	})
}

func (h *Handler) ProfilePasswordPost(c echo.Context) error {
	user := c.Get("user").(*models.User)

	redirect := func(key, msg string) error {
		return c.Redirect(http.StatusFound, "/profile?"+key+"="+msg)
	}

	current := c.FormValue("current_password")
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(current)); err != nil {
		return redirect("error", "Mot+de+passe+actuel+incorrect")
	}

	newPw := c.FormValue("new_password")
	confirm := c.FormValue("confirm_password")

	if len(newPw) < 6 {
		return redirect("error", "Le+mot+de+passe+doit+faire+au+moins+6+caractères")
	}
	if newPw != confirm {
		return redirect("error", "Les+mots+de+passe+ne+correspondent+pas")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPw), bcrypt.DefaultCost)
	if err != nil {
		return redirect("error", "Erreur+interne")
	}

	h.DB.Model(user).Updates(map[string]interface{}{
		"password_hash":        string(hash),
		"must_change_password": false,
	})

	return redirect("success", "Mot+de+passe+mis+à+jour")
}

