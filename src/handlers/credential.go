package handlers

import (
	"fmt"
	"gomme/crypto"
	"gomme/models"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

type CredentialView struct {
	models.Credential
	CanModify bool
}

type CredentialListData struct {
	User          *models.User
	Credentials   []CredentialView
	Organizations []models.Organization
	Success       string
	Error         string
}

// accessibleOrgIDs retourne les IDs des organisations accessibles à l'utilisateur.
func (h *Handler) accessibleOrgIDs(user *models.User) []uint {
	orgs := h.userOrgs(user.ID)
	ids := make([]uint, len(orgs))
	for i, o := range orgs {
		ids[i] = o.ID
	}
	return ids
}

// canModifyCredential vérifie si l'utilisateur peut modifier/supprimer un identifiant.
func (h *Handler) canModifyCredential(user *models.User, cred *models.Credential) bool {
	if user.IsAdmin {
		return true
	}
	if cred.OrganizationID == nil {
		return cred.UserID == user.ID
	}
	return h.checkOrgAccess(user, *cred.OrganizationID, "update_credential")
}

// canViewCredential vérifie si l'utilisateur peut voir un identifiant.
func (h *Handler) canViewCredential(user *models.User, cred *models.Credential) bool {
	if user.IsAdmin {
		return true
	}
	if cred.OrganizationID == nil {
		return cred.UserID == user.ID
	}
	member := h.getOrgMember(*cred.OrganizationID, user.ID)
	org := models.Organization{}
	h.DB.Select("owner_id").First(&org, *cred.OrganizationID)
	return org.OwnerID == user.ID || member != nil
}

func (h *Handler) CredentialList(c echo.Context) error {
	user := c.Get("user").(*models.User)
	data := CredentialListData{
		User:    user,
		Success: c.QueryParam("success"),
		Error:   c.QueryParam("error"),
	}

	var creds []models.Credential
	if user.IsAdmin {
		h.DB.Preload("Organization").Preload("Fields").Find(&creds)
	} else {
		orgIDs := h.accessibleOrgIDs(user)
		if len(orgIDs) == 0 {
			h.DB.Preload("Organization").Preload("Fields").
				Where("organization_id IS NULL AND user_id = ?", user.ID).
				Find(&creds)
		} else {
			h.DB.Preload("Organization").Preload("Fields").
				Where("(organization_id IS NULL AND user_id = ?) OR (organization_id IN ?)", user.ID, orgIDs).
				Find(&creds)
		}
	}
	for _, cred := range creds {
		data.Credentials = append(data.Credentials, CredentialView{
			Credential: cred,
			CanModify:  h.canModifyCredential(user, &cred),
		})
	}

	data.Organizations = h.userOrgs(user.ID)
	return c.Render(http.StatusOK, "credentials/list", data)
}

func (h *Handler) CredentialCreate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	name := c.FormValue("name")
	if name == "" {
		return c.Redirect(http.StatusFound, "/credentials?error=Nom+requis")
	}

	cred := models.Credential{
		Name:   name,
		UserID: user.ID,
	}

	if orgIDStr := c.FormValue("organization_id"); orgIDStr != "" {
		if orgID, err := strconv.ParseUint(orgIDStr, 10, 64); err == nil && orgID > 0 {
			if !h.checkOrgAccess(user, uint(orgID), "update_playbook") {
				return c.Redirect(http.StatusFound, "/credentials?error=Accès+refusé+à+cette+organisation")
			}
			oid := uint(orgID)
			cred.OrganizationID = &oid
		}
	}

	h.DB.Create(&cred)
	h.saveCredentialFields(cred.ID, c, nil)
	return c.Redirect(http.StatusFound, "/credentials?success=Identifiant+créé")
}

func (h *Handler) CredentialUpdate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var cred models.Credential
	if err := h.DB.Preload("Fields").First(&cred, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/credentials?error=Identifiant+introuvable")
	}
	if !h.canModifyCredential(user, &cred) {
		return c.Redirect(http.StatusFound, "/credentials?error=Accès+refusé")
	}

	name := c.FormValue("name")
	if name == "" {
		return c.Redirect(http.StatusFound, "/credentials?error=Nom+requis")
	}
	cred.Name = name

	if orgIDStr := c.FormValue("organization_id"); orgIDStr != "" {
		if orgID, err := strconv.ParseUint(orgIDStr, 10, 64); err == nil && orgID > 0 {
			if !h.checkOrgAccess(user, uint(orgID), "update_playbook") {
				return c.Redirect(http.StatusFound, "/credentials?error=Accès+refusé+à+cette+organisation")
			}
			oid := uint(orgID)
			cred.OrganizationID = &oid
		} else {
			cred.OrganizationID = nil
		}
	} else {
		cred.OrganizationID = nil
	}

	h.DB.Save(&cred)

	// Reconstruire les champs (conserver les valeurs chiffrées des champs secrets laissés vides)
	oldFields := map[string]string{}
	for _, f := range cred.Fields {
		oldFields[f.Key] = f.ValueEnc
	}
	h.DB.Where("credential_id = ?", cred.ID).Delete(&models.CredentialField{})
	h.saveCredentialFields(cred.ID, c, oldFields)

	return c.Redirect(http.StatusFound, fmt.Sprintf("/credentials?success=Identifiant+%s+mis+à+jour", cred.Name))
}

// saveCredentialFields crée les CredentialField depuis les paramètres du formulaire.
// oldFields : map[key]valueEnc existants (pour conserver un secret non modifié).
func (h *Handler) saveCredentialFields(credID uint, c echo.Context, oldFields map[string]string) {
	params, _ := c.FormParams()
	keys := params["field_key"]
	values := params["field_value"]
	secrets := params["field_secret"] // "0" ou "1" par champ

	for i, key := range keys {
		if key == "" {
			continue
		}
		val := ""
		if i < len(values) {
			val = values[i]
		}
		isSecret := i < len(secrets) && secrets[i] == "1"

		var enc string
		if val != "" {
			if e, err := crypto.Encrypt(h.Config.SecretKey, val); err == nil {
				enc = e
			}
		} else if isSecret && oldFields != nil {
			// Champ secret laissé vide → conserver l'ancienne valeur chiffrée
			enc = oldFields[key]
		}

		h.DB.Create(&models.CredentialField{
			CredentialID: credID,
			Key:          key,
			ValueEnc:     enc,
			Secret:       isSecret,
		})
	}
}

func (h *Handler) CredentialDelete(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var cred models.Credential
	if err := h.DB.First(&cred, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/credentials?error=Identifiant+introuvable")
	}
	if !h.canModifyCredential(user, &cred) {
		return c.Redirect(http.StatusFound, "/credentials?error=Accès+refusé")
	}

	h.DB.Where("credential_id = ?", cred.ID).Delete(&models.CredentialField{})
	h.DB.Table("playbook_credential_links").Where("credential_id = ?", cred.ID).Delete(nil)
	h.DB.Delete(&cred)
	return c.Redirect(http.StatusFound, "/credentials?success=Identifiant+supprimé")
}

// CredentialFieldsAPI retourne les champs d'un identifiant pour la modale d'édition.
// Les valeurs des champs secrets ne sont jamais retournées (chaîne vide).
func (h *Handler) CredentialFieldsAPI(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var cred models.Credential
	if err := h.DB.Preload("Fields").First(&cred, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "introuvable"})
	}
	if !h.canViewCredential(user, &cred) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "accès refusé"})
	}

	type fieldResp struct {
		Key    string `json:"key"`
		Value  string `json:"value"`
		Secret bool   `json:"secret"`
	}
	resp := make([]fieldResp, 0, len(cred.Fields))
	for _, f := range cred.Fields {
		val := ""
		if !f.Secret && f.ValueEnc != "" {
			if dec, err := crypto.Decrypt(h.Config.SecretKey, f.ValueEnc); err == nil {
				val = dec
			}
		}
		resp = append(resp, fieldResp{Key: f.Key, Value: val, Secret: f.Secret})
	}
	return c.JSON(http.StatusOK, resp)
}
