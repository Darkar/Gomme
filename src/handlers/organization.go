package handlers

import (
	"fmt"
	"gomme/models"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// getOrgMember retourne l'entrée OrganizationMember si l'user est dans l'org, nil sinon.
func (h *Handler) getOrgMember(orgID, userID uint) *models.OrganizationMember {
	var m models.OrganizationMember
	if h.DB.Where("organization_id = ? AND user_id = ?", orgID, userID).First(&m).Error != nil {
		return nil
	}
	return &m
}

// isOrgOwner retourne true si l'user est propriétaire de l'org.
func (h *Handler) isOrgOwner(orgID, userID uint) bool {
	var org models.Organization
	if h.DB.Select("owner_id").First(&org, orgID).Error != nil {
		return false
	}
	return org.OwnerID == userID
}

// checkOrgAccess vérifie si l'user peut effectuer une action sur une ressource de l'org.
func (h *Handler) checkOrgAccess(user *models.User, orgID uint, action string) bool {
	if user.IsAdmin {
		return true
	}
	if h.isOrgOwner(orgID, user.ID) {
		return true
	}
	m := h.getOrgMember(orgID, user.ID)
	if m == nil {
		return false
	}
	switch action {
	case "create_playbook":
		return m.CanCreatePlaybook
	case "update_playbook":
		return m.CanUpdatePlaybook
	case "delete_playbook":
		return m.CanDeletePlaybook
	case "create_inventory":
		return m.CanCreateInventory
	case "update_inventory":
		return m.CanUpdateInventory
	case "delete_inventory":
		return m.CanDeleteInventory
	case "create_credential":
		return m.CanCreateCredential
	case "update_credential":
		return m.CanUpdateCredential
	case "delete_credential":
		return m.CanDeleteCredential
	case "create_repository":
		return m.CanCreateRepository
	case "update_repository":
		return m.CanUpdateRepository
	case "delete_repository":
		return m.CanDeleteRepository
	}
	return false
}

// userOrgs retourne les orgs dont l'user est owner OU membre.
func (h *Handler) userOrgs(userID uint) []models.Organization {
	var orgs []models.Organization
	h.DB.Preload("Owner").Where("owner_id = ?", userID).Find(&orgs)

	var memberOrgs []models.Organization
	h.DB.Preload("Owner").
		Joins("JOIN organization_members ON organization_members.organization_id = organizations.id").
		Where("organization_members.user_id = ? AND organizations.owner_id != ?", userID, userID).
		Find(&memberOrgs)

	return append(orgs, memberOrgs...)
}

// ─── Data structs ─────────────────────────────────────────────────────────────

// permLevel dérive un niveau de permission lisible à partir de 3 bits C/M/S.
func permLevel(c, u, d bool) string {
	if d {
		return "manage"
	}
	if u {
		return "edit"
	}
	if c {
		return "create"
	}
	return "none"
}

// permBits convertit un niveau de permission en 3 bits C/M/S.
func permBits(val string) (create, update, del bool) {
	switch val {
	case "create":
		return true, false, false
	case "edit":
		return true, true, false
	case "manage":
		return true, true, true
	}
	return false, false, false
}

// OrgMemberView embarque OrganizationMember avec les niveaux de permission calculés.
type OrgMemberView struct {
	models.OrganizationMember
	PermPlaybook   string
	PermInventory  string
	PermCredential string
	PermRepository string
}

type OrgListData struct {
	User          *models.User
	Organizations []OrgSummary
	Success       string
	Error         string
}

type OrgSummary struct {
	models.Organization
	MemberCount int64
	IsOwner     bool
}

type OrgDetailData struct {
	User    *models.User
	Org     models.Organization
	Members []OrgMemberView
	Users   []models.User // users non encore membres (pour le sélecteur d'ajout)
	IsOwner bool
	Success string
	Error   string
}

// ─── Handlers utilisateur ────────────────────────────────────────────────────

func (h *Handler) OrganizationList(c echo.Context) error {
	user := c.Get("user").(*models.User)
	data := OrgListData{
		User:    user,
		Success: c.QueryParam("success"),
		Error:   c.QueryParam("error"),
	}
	for _, org := range h.userOrgs(user.ID) {
		var count int64
		h.DB.Model(&models.OrganizationMember{}).Where("organization_id = ?", org.ID).Count(&count)
		data.Organizations = append(data.Organizations, OrgSummary{
			Organization: org,
			MemberCount:  count,
			IsOwner:      org.OwnerID == user.ID,
		})
	}
	return c.Render(http.StatusOK, "organization/list", data)
}

func (h *Handler) OrganizationCreate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	name := c.FormValue("name")
	if name == "" {
		return c.Redirect(http.StatusFound, "/organizations?error=Nom+requis")
	}
	org := models.Organization{Name: name, OwnerID: user.ID}
	if err := h.DB.Create(&org).Error; err != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations?error=%s", err.Error()))
	}
	// Le propriétaire est automatiquement membre avec tous les droits
	h.DB.Create(&models.OrganizationMember{
		OrganizationID:      org.ID,
		UserID:              user.ID,
		CanCreatePlaybook:   true,
		CanUpdatePlaybook:   true,
		CanDeletePlaybook:   true,
		CanCreateInventory:  true,
		CanUpdateInventory:  true,
		CanDeleteInventory:  true,
		CanCreateCredential: true,
		CanUpdateCredential: true,
		CanDeleteCredential: true,
		CanCreateRepository: true,
		CanUpdateRepository: true,
		CanDeleteRepository: true,
	})
	return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?success=Organisation+créée", org.ID))
}

func (h *Handler) OrganizationDetail(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var org models.Organization
	if err := h.DB.Preload("Owner").First(&org, id).Error; err != nil {
		return echo.ErrNotFound
	}

	// Vérifier que l'user est membre ou admin
	isOwner := org.OwnerID == user.ID
	isMember := isOwner || user.IsAdmin || h.getOrgMember(org.ID, user.ID) != nil
	if !isMember {
		return echo.ErrForbidden
	}

	data := OrgDetailData{
		User:    user,
		Org:     org,
		IsOwner: isOwner || user.IsAdmin,
		Success: c.QueryParam("success"),
		Error:   c.QueryParam("error"),
	}
	var rawMembers []models.OrganizationMember
	h.DB.Preload("User").Where("organization_id = ?", org.ID).Find(&rawMembers)
	for _, m := range rawMembers {
		data.Members = append(data.Members, OrgMemberView{
			OrganizationMember: m,
			PermPlaybook:       permLevel(m.CanCreatePlaybook, m.CanUpdatePlaybook, m.CanDeletePlaybook),
			PermInventory:      permLevel(m.CanCreateInventory, m.CanUpdateInventory, m.CanDeleteInventory),
			PermCredential:     permLevel(m.CanCreateCredential, m.CanUpdateCredential, m.CanDeleteCredential),
			PermRepository:     permLevel(m.CanCreateRepository, m.CanUpdateRepository, m.CanDeleteRepository),
		})
	}

	// Charger les users qui ne sont pas encore membres (pour le sélecteur d'ajout)
	var memberIDs []uint
	for _, m := range data.Members {
		memberIDs = append(memberIDs, m.UserID)
	}
	if len(memberIDs) > 0 {
		h.DB.Where("id NOT IN ?", memberIDs).Find(&data.Users)
	} else {
		h.DB.Find(&data.Users)
	}

	return c.Render(http.StatusOK, "organization/detail", data)
}

func (h *Handler) OrganizationUpdate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var org models.Organization
	if err := h.DB.First(&org, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/organizations?error=Organisation+introuvable")
	}
	if !user.IsAdmin && org.OwnerID != user.ID {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Accès+refusé", id))
	}
	name := c.FormValue("name")
	if name == "" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Nom+requis", id))
	}
	org.Name = name
	h.DB.Save(&org)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?success=Organisation+mise+à+jour", id))
}

func (h *Handler) OrganizationDelete(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var org models.Organization
	if err := h.DB.First(&org, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/organizations?error=Organisation+introuvable")
	}
	if !user.IsAdmin && org.OwnerID != user.ID {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Accès+refusé", id))
	}
	h.DB.Model(&models.Inventory{}).Where("organization_id = ?", id).Update("organization_id", nil)
	h.DB.Model(&models.Playbook{}).Where("organization_id = ?", id).Update("organization_id", nil)
	h.DB.Model(&models.Credential{}).Where("organization_id = ?", id).Update("organization_id", nil)
	h.DB.Model(&models.Repository{}).Where("organization_id = ?", id).Update("organization_id", nil)
	h.DB.Where("organization_id = ?", id).Delete(&models.OrganizationMember{})
	h.DB.Delete(&org)
	return c.Redirect(http.StatusFound, "/organizations?success=Organisation+supprimée")
}

func (h *Handler) OrganizationTransfer(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var org models.Organization
	if err := h.DB.First(&org, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/organizations?error=Organisation+introuvable")
	}
	if org.OwnerID != user.ID && !user.IsAdmin {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Accès+refusé", id))
	}
	newOwnerID, err := strconv.ParseUint(c.FormValue("new_owner_id"), 10, 64)
	if err != nil || newOwnerID == 0 {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Utilisateur+invalide", id))
	}
	// Vérifier que le nouveau propriétaire est déjà membre
	if h.getOrgMember(org.ID, uint(newOwnerID)) == nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Le+nouvel+propriétaire+doit+être+membre", id))
	}
	org.OwnerID = uint(newOwnerID)
	h.DB.Save(&org)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?success=Propriété+transférée", id))
}

func (h *Handler) OrganizationMemberAdd(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var org models.Organization
	if err := h.DB.First(&org, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/organizations?error=Organisation+introuvable")
	}
	if !user.IsAdmin && org.OwnerID != user.ID {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Accès+refusé", id))
	}
	memberID, err := strconv.ParseUint(c.FormValue("user_id"), 10, 64)
	if err != nil || memberID == 0 {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Utilisateur+invalide", id))
	}
	var target models.User
	if h.DB.First(&target, memberID).Error != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Utilisateur+introuvable", id))
	}
	if h.getOrgMember(org.ID, uint(memberID)) != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Utilisateur+déjà+membre", id))
	}
	pbC, pbU, pbD := permBits(c.FormValue("perm_playbook"))
	invC, invU, invD := permBits(c.FormValue("perm_inventory"))
	credC, credU, credD := permBits(c.FormValue("perm_credential"))
	repoC, repoU, repoD := permBits(c.FormValue("perm_repository"))
	member := models.OrganizationMember{
		OrganizationID:      org.ID,
		UserID:              uint(memberID),
		CanCreatePlaybook:   pbC,
		CanUpdatePlaybook:   pbU,
		CanDeletePlaybook:   pbD,
		CanCreateInventory:  invC,
		CanUpdateInventory:  invU,
		CanDeleteInventory:  invD,
		CanCreateCredential: credC,
		CanUpdateCredential: credU,
		CanDeleteCredential: credD,
		CanCreateRepository: repoC,
		CanUpdateRepository: repoU,
		CanDeleteRepository: repoD,
	}
	h.DB.Create(&member)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?success=Membre+ajouté", id))
}

func (h *Handler) OrganizationMemberUpdate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	uid, _ := strconv.ParseUint(c.Param("uid"), 10, 64)

	var org models.Organization
	if err := h.DB.First(&org, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/organizations?error=Organisation+introuvable")
	}
	if !user.IsAdmin && org.OwnerID != user.ID {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Accès+refusé", id))
	}
	// Empêcher de modifier les droits du propriétaire (il a toujours tous les droits)
	if uint(uid) == org.OwnerID {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Impossible+de+modifier+les+droits+du+propriétaire", id))
	}
	var member models.OrganizationMember
	if h.DB.Where("organization_id = ? AND user_id = ?", id, uid).First(&member).Error != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Membre+introuvable", id))
	}
	pbC, pbU, pbD := permBits(c.FormValue("perm_playbook"))
	invC, invU, invD := permBits(c.FormValue("perm_inventory"))
	credC, credU, credD := permBits(c.FormValue("perm_credential"))
	repoC, repoU, repoD := permBits(c.FormValue("perm_repository"))
	member.CanCreatePlaybook = pbC
	member.CanUpdatePlaybook = pbU
	member.CanDeletePlaybook = pbD
	member.CanCreateInventory = invC
	member.CanUpdateInventory = invU
	member.CanDeleteInventory = invD
	member.CanCreateCredential = credC
	member.CanUpdateCredential = credU
	member.CanDeleteCredential = credD
	member.CanCreateRepository = repoC
	member.CanUpdateRepository = repoU
	member.CanDeleteRepository = repoD
	h.DB.Save(&member)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?success=Permissions+mises+à+jour", id))
}

func (h *Handler) OrganizationMemberRemove(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	uid, _ := strconv.ParseUint(c.Param("uid"), 10, 64)

	var org models.Organization
	if err := h.DB.First(&org, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/organizations?error=Organisation+introuvable")
	}
	if !user.IsAdmin && org.OwnerID != user.ID {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Accès+refusé", id))
	}
	// Refuser de retirer le propriétaire
	if uint(uid) == org.OwnerID {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?error=Impossible+de+retirer+le+propriétaire", id))
	}
	h.DB.Where("organization_id = ? AND user_id = ?", id, uid).Delete(&models.OrganizationMember{})
	return c.Redirect(http.StatusFound, fmt.Sprintf("/organizations/%d?success=Membre+retiré", id))
}

