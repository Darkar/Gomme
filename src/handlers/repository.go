package handlers

import (
	"fmt"
	"gomme/crypto"
	"gomme/models"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type RepositoryView struct {
	models.Repository
	CanEdit   bool
	CanDelete bool
}

type RepositoryListData struct {
	User         *models.User
	Repositories []RepositoryView
	Success      string
	Error        string
}

func (h *Handler) RepositoryList(c echo.Context) error {
	user := c.Get("user").(*models.User)
	data := RepositoryListData{
		User:    user,
		Success: c.QueryParam("success"),
		Error:   c.QueryParam("error"),
	}
	var repos []models.Repository
	h.DB.Find(&repos)
	for _, repo := range repos {
		canEdit := repo.OrganizationID == nil || h.checkOrgAccess(user, *repo.OrganizationID, "update_repository")
		canDelete := repo.OrganizationID == nil || h.checkOrgAccess(user, *repo.OrganizationID, "delete_repository")
		data.Repositories = append(data.Repositories, RepositoryView{Repository: repo, CanEdit: canEdit, CanDelete: canDelete})
	}
	return c.Render(http.StatusOK, "repository/list", data)
}

func (h *Handler) RepositoryCreate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	repo := models.Repository{
		Name:        c.FormValue("name"),
		URL:         c.FormValue("url"),
		Branch:      c.FormValue("branch"),
		AutoSync:    c.FormValue("auto_sync") == "on",
		InsecureTLS: c.FormValue("insecure_tls") == "on",
		AuthType:    c.FormValue("auth_type"),
		Username:    c.FormValue("username"),
		UserID:      user.ID,
	}
	if repo.Branch == "" {
		repo.Branch = "main"
	}
	if repo.AuthType == "" {
		repo.AuthType = "none"
	}
	if repo.Name == "" || repo.URL == "" {
		return c.Redirect(http.StatusFound, "/repository?error=Nom+et+URL+requis")
	}

	if orgIDStr := c.FormValue("organization_id"); orgIDStr != "" {
		if orgID, err := strconv.ParseUint(orgIDStr, 10, 64); err == nil && orgID > 0 {
			if !h.checkOrgAccess(user, uint(orgID), "create_repository") {
				return c.Redirect(http.StatusFound, "/repository?error=Accès+refusé+à+cette+organisation")
			}
			oid := uint(orgID)
			repo.OrganizationID = &oid
		}
	}

	if err := h.encryptRepoCredentials(&repo, c, "", ""); err != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/repository?error=%s", err.Error()))
	}

	h.DB.Create(&repo)
	return c.Redirect(http.StatusFound, "/repository?success=Repository+créé")
}

func (h *Handler) RepositoryUpdate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var repo models.Repository
	if err := h.DB.First(&repo, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/repository?error=Repository+introuvable")
	}
	if repo.OrganizationID != nil && !h.checkOrgAccess(user, *repo.OrganizationID, "update_repository") {
		return c.Redirect(http.StatusFound, "/repository?error=Accès+refusé")
	}

	existingPasswordEnc := repo.PasswordEnc
	existingSSHKeyEnc := repo.SSHKeyEnc

	repo.Name = c.FormValue("name")
	repo.URL = c.FormValue("url")
	repo.Branch = c.FormValue("branch")
	repo.AutoSync = c.FormValue("auto_sync") == "on"
	repo.InsecureTLS = c.FormValue("insecure_tls") == "on"
	repo.AuthType = c.FormValue("auth_type")
	repo.Username = c.FormValue("username")
	if repo.Branch == "" {
		repo.Branch = "main"
	}
	if repo.AuthType == "" {
		repo.AuthType = "none"
	}

	if err := h.encryptRepoCredentials(&repo, c, existingPasswordEnc, existingSSHKeyEnc); err != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/repository?error=%s", err.Error()))
	}

	h.DB.Save(&repo)
	return c.Redirect(http.StatusFound, "/repository?success=Repository+mis+à+jour")
}

// encryptRepoCredentials chiffre le mot de passe ou la clé SSH depuis le formulaire.
// Les valeurs existingPasswordEnc / existingSSHKeyEnc sont conservées si les champs sont vides.
func (h *Handler) encryptRepoCredentials(repo *models.Repository, c echo.Context, existingPasswordEnc, existingSSHKeyEnc string) error {
	switch repo.AuthType {
	case "password":
		repo.SSHKeyEnc = ""
		if pwd := c.FormValue("password"); pwd != "" {
			enc, err := crypto.Encrypt(h.Config.SecretKey, pwd)
			if err != nil {
				return fmt.Errorf("chiffrement mot de passe: %w", err)
			}
			repo.PasswordEnc = enc
		} else {
			repo.PasswordEnc = existingPasswordEnc
		}
	case "ssh_key":
		repo.PasswordEnc = ""
		if key := c.FormValue("ssh_key"); key != "" {
			enc, err := crypto.Encrypt(h.Config.SecretKey, key)
			if err != nil {
				return fmt.Errorf("chiffrement clé SSH: %w", err)
			}
			repo.SSHKeyEnc = enc
		} else {
			repo.SSHKeyEnc = existingSSHKeyEnc
		}
	default:
		repo.PasswordEnc = ""
		repo.SSHKeyEnc = ""
	}
	return nil
}

func (h *Handler) RepositoryDelete(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var repo models.Repository
	if err := h.DB.First(&repo, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/repository?error=Repository+introuvable")
	}
	if repo.OrganizationID != nil && !h.checkOrgAccess(user, *repo.OrganizationID, "delete_repository") {
		return c.Redirect(http.StatusFound, "/repository?error=Accès+refusé")
	}
	h.DB.Where("repository_id = ?", id).Delete(&models.Playbook{})
	h.DB.Delete(&repo)
	if repo.LocalPath != "" {
		os.RemoveAll(repo.LocalPath)
	}
	return c.Redirect(http.StatusFound, "/repository?success=Repository+supprimé")
}

func (h *Handler) RepositorySync(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var repo models.Repository
	if err := h.DB.First(&repo, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/repository?error=Repository+introuvable")
	}

	if err := h.syncRepo(&repo); err != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/repository?error=Sync+échouée+%%3A+%s", err.Error()))
	}

	h.DB.Save(&repo)

	return c.Redirect(http.StatusFound, "/repository?success=Repository+synchronisé")
}

func (h *Handler) syncRepo(repo *models.Repository) error {
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, repo.Name)
	repoPath := filepath.Join("repo", fmt.Sprintf("%d_%s", repo.ID, safeName))

	// URL sans credentials (pour .git/config) et URL avec credentials (pour les commandes git)
	plainURL := repo.URL
	authURL, sshEnv, sshCleanup, err := h.buildAuthContext(repo)
	if err != nil {
		return err
	}
	if sshCleanup != nil {
		defer sshCleanup()
	}

	runGit := func(args ...string) error {
		cmd := exec.Command("git", args...)
		env := sshEnv
		if repo.InsecureTLS {
			if env == nil {
				env = os.Environ()
			}
			env = append(env, "GIT_SSL_NO_VERIFY=true")
		}
		if env != nil {
			cmd.Env = env
		}
		if output, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
			msg := strings.TrimSpace(string(output))
			if msg == "" {
				msg = cmdErr.Error()
			}
			return fmt.Errorf("%s", msg)
		}
		return nil
	}

	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		if err := os.MkdirAll("repo", 0755); err != nil {
			return err
		}
		// Clone avec l'URL authentifiée
		if err := runGit("clone", "--branch", repo.Branch, authURL, repoPath); err != nil {
			return err
		}
		// Retirer immédiatement les credentials de .git/config
		if authURL != plainURL {
			exec.Command("git", "-C", repoPath, "remote", "set-url", "origin", plainURL).Run()
		}
	} else {
		// Pull : on passe l'URL authentifiée directement pour ne pas modifier .git/config
		if err := runGit("-C", repoPath, "pull", authURL, repo.Branch); err != nil {
			return err
		}
	}

	repo.LocalPath = repoPath
	now := time.Now()
	repo.LastSyncAt = &now
	return nil
}

// buildAuthContext retourne :
//   - authURL : URL avec credentials intégrés (HTTPS) ou URL brute (SSH/none)
//   - sshEnv  : variables d'environnement pour GIT_SSH_COMMAND (SSH uniquement)
//   - cleanup : supprime les fichiers temporaires (clé SSH)
func (h *Handler) buildAuthContext(repo *models.Repository) (authURL string, sshEnv []string, cleanup func(), err error) {
	authURL = repo.URL

	switch repo.AuthType {
	case "password":
		if repo.PasswordEnc == "" {
			return
		}
		password, decErr := crypto.Decrypt(h.Config.SecretKey, repo.PasswordEnc)
		if decErr != nil {
			err = fmt.Errorf("déchiffrement mot de passe: %w", decErr)
			return
		}
		u, parseErr := url.Parse(repo.URL)
		if parseErr != nil {
			err = fmt.Errorf("URL invalide: %w", parseErr)
			return
		}
		u.User = url.UserPassword(repo.Username, password)
		authURL = u.String()

	case "ssh_key":
		if repo.SSHKeyEnc == "" {
			return
		}
		sshKey, decErr := crypto.Decrypt(h.Config.SecretKey, repo.SSHKeyEnc)
		if decErr != nil {
			err = fmt.Errorf("déchiffrement clé SSH: %w", decErr)
			return
		}
		tmp, tmpErr := os.CreateTemp("", "gomme-key-*")
		if tmpErr != nil {
			err = fmt.Errorf("fichier clé SSH temporaire: %w", tmpErr)
			return
		}
		tmp.WriteString(sshKey)
		tmp.Close()
		os.Chmod(tmp.Name(), 0600)
		sshCmd := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", tmp.Name())
		sshEnv = append(os.Environ(), "GIT_SSH_COMMAND="+sshCmd)
		cleanup = func() { os.Remove(tmp.Name()) }
	}
	return
}

// extractPlaybookName lit le fichier YAML et retourne la valeur de la clé "name"
// du premier play (ligne "- name: ..." ou "name: ..."). Retourne "" si non trouvée.
func extractPlaybookName(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- ") {
			t = strings.TrimSpace(t[2:])
		}
		if strings.HasPrefix(t, "name:") {
			name := strings.TrimSpace(t[5:])
			name = strings.Trim(name, `"'`)
			if name != "" {
				return name
			}
		}
	}
	return ""
}

// RepositoryFiles retourne la liste des fichiers YAML dans un repository synchronisé.
func (h *Handler) RepositoryFiles(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var repo models.Repository
	if err := h.DB.First(&repo, id).Error; err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"files": []string{}})
	}
	if repo.LocalPath == "" {
		return c.JSON(http.StatusOK, map[string]interface{}{"files": []string{}})
	}
	var files []string
	filepath.Walk(repo.LocalPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
			rel, _ := filepath.Rel(repo.LocalPath, path)
			files = append(files, rel)
		}
		return nil
	})
	if files == nil {
		files = []string{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"files": files})
}
