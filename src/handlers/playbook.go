package handlers

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"gomme/crypto"
	"gomme/docker"
	"gomme/models"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type PlaybookView struct {
	models.Playbook
	CanEdit bool
}

type PlaybookListData struct {
	User          *models.User
	Playbooks     []PlaybookView
	Organizations []models.Organization
	Repositories  []models.Repository
	Inventories   []models.Inventory
	Success       string
	Error         string
}

type PlaybookVarsData struct {
	User     *models.User
	Playbook models.Playbook
	Vars     []models.PlaybookVar
	Success  string
	Error    string
}

type PlaybookSurveyData struct {
	User         *models.User
	Playbook     models.Playbook
	SurveyFields []models.SurveyField
	Success      string
}

type PlaybookEditData struct {
	User                 *models.User
	Playbook             models.Playbook
	LinkedInventories    []models.PlaybookInventoryLink
	Vars                 []models.PlaybookVar
	SurveyFields         []models.SurveyField
	AvailableCredentials []models.Credential
	LinkedCredentialIDs  map[uint]bool
	Organizations        []models.Organization
	Repositories         []models.Repository
	Inventories          []models.Inventory // inventaires accessibles pour ajout
	DockerImages         []models.ExecutionImage
	RepoFiles            []string
	OrganizationID       uint
	ScheduledTasks       []models.ScheduledTask
	Tab                  string
	Success              string
	Error                string
}

type RunDetailData struct {
	User *models.User
	Run  models.PlaybookRun
}

// RunInventory est utilisé en interne pour passer les inventaires à executePlaybook.
type RunInventory struct {
	InventoryID uint
	GroupFilter string
}

func (h *Handler) PlaybookList(c echo.Context) error {
	user := c.Get("user").(*models.User)
	data := PlaybookListData{
		User:    user,
		Success: c.QueryParam("success"),
		Error:   c.QueryParam("error"),
	}
	var playbooks []models.Playbook
	h.DB.Preload("Repository").Preload("Organization").Preload("SurveyFields").Find(&playbooks)
	for _, pb := range playbooks {
		canEdit := pb.OrganizationID == nil || h.checkOrgAccess(user, *pb.OrganizationID, "update_playbook")
		data.Playbooks = append(data.Playbooks, PlaybookView{Playbook: pb, CanEdit: canEdit})
	}
	data.Organizations = h.userOrgs(user.ID)
	h.DB.Find(&data.Repositories)
	// Inventaires accessibles pour la sélection au lancement
	var allInvs []models.Inventory
	h.DB.Find(&allInvs)
	for _, inv := range allInvs {
		if inv.OrganizationID == nil || user.IsAdmin || h.checkOrgAccess(user, *inv.OrganizationID, "update_inventory") {
			data.Inventories = append(data.Inventories, inv)
		}
	}
	return c.Render(http.StatusOK, "playbooks/list", data)
}

func (h *Handler) PlaybookCreate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	name := c.FormValue("name")
	path := c.FormValue("path")
	repoIDStr := c.FormValue("repository_id")
	if name == "" || path == "" || repoIDStr == "" {
		return c.Redirect(http.StatusFound, "/playbooks?error=Nom%2C+repository+et+chemin+requis")
	}
	repoID, err := strconv.ParseUint(repoIDStr, 10, 64)
	if err != nil || repoID == 0 {
		return c.Redirect(http.StatusFound, "/playbooks?error=Repository+invalide")
	}
	pb := models.Playbook{
		Name:         name,
		RepositoryID: uint(repoID),
		Path:         path,
		UserID:       user.ID,
	}
	h.DB.Create(&pb)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?success=Playbook+créé", pb.ID))
}

func (h *Handler) PlaybookDelete(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var pb models.Playbook
	if err := h.DB.First(&pb, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/playbooks?error=Playbook+introuvable")
	}
	if pb.OrganizationID != nil && !h.checkOrgAccess(user, *pb.OrganizationID, "delete_playbook") {
		return c.Redirect(http.StatusFound, "/playbooks?error=Accès+refusé")
	}
	h.DB.Where("playbook_id = ?", id).Delete(&models.PlaybookVar{})
	h.DB.Where("playbook_id = ?", id).Delete(&models.SurveyField{})
	h.DB.Where("playbook_id = ?", id).Delete(&models.PlaybookInventoryLink{})
	h.DB.Delete(&pb)
	return c.Redirect(http.StatusFound, "/playbooks?success=Playbook+supprimé")
}

func (h *Handler) PlaybookUpdate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var pb models.Playbook
	if err := h.DB.First(&pb, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/playbooks?error=Playbook+introuvable")
	}
	if pb.OrganizationID != nil && !h.checkOrgAccess(user, *pb.OrganizationID, "update_playbook") {
		return c.Redirect(http.StatusFound, "/playbooks?error=Accès+refusé")
	}

	pb.Name = c.FormValue("name")
	pb.Description = c.FormValue("description")
	pb.Path = c.FormValue("path")
	pb.DockerImage = c.FormValue("docker_image")
	pb.DefaultLimit = c.FormValue("default_limit")

	if repoIDStr := c.FormValue("repository_id"); repoIDStr != "" {
		if rid, err := strconv.ParseUint(repoIDStr, 10, 64); err == nil && rid > 0 {
			pb.RepositoryID = uint(rid)
		}
	}

	if orgIDStr := c.FormValue("organization_id"); orgIDStr != "" {
		if orgID, err := strconv.ParseUint(orgIDStr, 10, 64); err == nil && orgID > 0 {
			if !h.checkOrgAccess(user, uint(orgID), "update_playbook") {
				return c.Redirect(http.StatusFound, "/playbooks?error=Accès+refusé")
			}
			oid := uint(orgID)
			pb.OrganizationID = &oid
		} else {
			pb.OrganizationID = nil
		}
	} else {
		pb.OrganizationID = nil
	}

	h.DB.Save(&pb)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?success=Playbook+mis+à+jour", id))
}

func (h *Handler) PlaybookDetail(c echo.Context) error {
	id := c.Param("id")
	return c.Redirect(http.StatusFound, "/playbooks/"+id+"/edit")
}

func (h *Handler) PlaybookSurveyAPI(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var fields []models.SurveyField
	h.DB.Where("playbook_id = ?", id).Order("sort_order").Find(&fields)
	if fields == nil {
		fields = []models.SurveyField{}
	}
	return c.JSON(http.StatusOK, fields)
}

// PlaybookInventoriesAPI retourne les inventaires liés au playbook (pour le modal de lancement).
func (h *Handler) PlaybookInventoriesAPI(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var links []models.PlaybookInventoryLink
	h.DB.Preload("Inventory").Where("playbook_id = ?", id).Find(&links)
	if links == nil {
		links = []models.PlaybookInventoryLink{}
	}
	return c.JSON(http.StatusOK, links)
}

func (h *Handler) PlaybookRun(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var pb models.Playbook
	if err := h.DB.Preload("Repository").Preload("Vars").Preload("SurveyFields").Preload("Credentials.Fields").Preload("Inventories").First(&pb, id).Error; err != nil {
		return echo.ErrNotFound
	}

	if pb.DockerImage == "" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?error=Image+Docker+non+configurée", id))
	}

	// Utiliser les inventaires configurés sur le playbook
	var runInvs []RunInventory
	for _, link := range pb.Inventories {
		runInvs = append(runInvs, RunInventory{
			InventoryID: link.InventoryID,
			GroupFilter: link.GroupFilter,
		})
	}

	now := time.Now()
	run := models.PlaybookRun{
		PlaybookID:  pb.ID,
		UserID:      user.ID,
		Limit:       pb.DefaultLimit,
		DockerImage: pb.DockerImage,
		Status:      "running",
		StartedAt:   &now,
	}
	if err := h.DB.Create(&run).Error; err != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?error=%s", id, url.QueryEscape("Impossible de créer l'exécution : "+err.Error())))
	}

	// Enregistrer les inventaires du run
	for _, ri := range runInvs {
		h.DB.Create(&models.PlaybookRunInventory{
			RunID:       run.ID,
			InventoryID: ri.InventoryID,
			GroupFilter: ri.GroupFilter,
		})
	}

	surveyVars := map[string]string{}
	for _, f := range pb.SurveyFields {
		surveyVars[f.VarName] = c.FormValue("survey_" + f.VarName)
	}

	runID := run.ID
	go func() {
		output, _, err := h.executePlaybook(&pb, runInvs, pb.DefaultLimit, pb.DockerImage, surveyVars, runID)
		status := "success"
		if err != nil {
			status = "failed"
			output += "\n\nErreur: " + err.Error()
		}
		finished := time.Now()
		h.DB.Model(&models.PlaybookRun{ID: runID}).Updates(map[string]interface{}{
			"status":      status,
			"output":      output,
			"finished_at": &finished,
		})
	}()

	return c.Redirect(http.StatusFound, fmt.Sprintf("/runs/%d", run.ID))
}

func (h *Handler) executePlaybook(pb *models.Playbook, invs []RunInventory, limit string, dockerImage string, surveyVars map[string]string, runID uint) (output string, containerID string, err error) {
	if h.Docker == nil {
		return "", "", fmt.Errorf("client Docker non disponible")
	}

	repoPath := pb.Repository.LocalPath
	if repoPath == "" {
		return "", "", fmt.Errorf("repository non synchronisé")
	}

	playbookPath := filepath.Join(filepath.Base(repoPath), pb.Path)

	// Construire les extra-vars Ansible : variables du playbook en base, survey en priorité
	allVars := make(map[string]string)
	for _, v := range pb.Vars {
		val := v.Value
		if v.Encrypted {
			if dec, decErr := crypto.Decrypt(h.Config.SecretKey, v.Value); decErr == nil {
				val = dec
			}
		}
		allVars[v.Key] = val
	}
	for k, v := range surveyVars {
		allVars[k] = v
	}

	// Champs des identifiants → variables Ansible (champs secrets masqués dans les logs)
	var secretValues []string
	for _, cred := range pb.Credentials {
		for _, field := range cred.Fields {
			if field.ValueEnc == "" {
				continue
			}
			dec, decErr := crypto.Decrypt(h.Config.SecretKey, field.ValueEnc)
			if decErr != nil {
				continue
			}
			allVars[field.Key] = dec
			if field.Secret {
				secretValues = append(secretValues, dec)
			}
		}
	}

	var env []string
	env = append(env, "ANSIBLE_HOST_KEY_CHECKING=False")
	env = append(env, "ANSIBLE_FORCE_COLOR=1")

	// Générer les fichiers inventaires (un par inventaire)
	var invFlags string
	if len(invs) == 0 {
		// Aucun inventaire → localhost
		env = append(env, "GOMME_INVENTORY_0=[all]\nlocalhost ansible_connection=local\n")
		invFlags = "-i /tmp/inv_0.ini"
	} else {
		for i, ri := range invs {
			content, buildErr := h.buildInventoryContent(ri.InventoryID, ri.GroupFilter)
			if buildErr != nil {
				return "", "", fmt.Errorf("génération inventaire %d: %w", ri.InventoryID, buildErr)
			}
			env = append(env, fmt.Sprintf("GOMME_INVENTORY_%d=%s", i, content))
			invFlags += fmt.Sprintf(" -i /tmp/inv_%d.ini", i)
		}
		invFlags = strings.TrimSpace(invFlags)
	}

	// Script : écrire les fichiers inventaire puis lancer ansible-playbook
	var scriptParts []string
	count := len(invs)
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		scriptParts = append(scriptParts, fmt.Sprintf("printf '%%s' \"$GOMME_INVENTORY_%d\" > /tmp/inv_%d.ini", i, i))
	}

	ansibleCmd := fmt.Sprintf("ansible-playbook %s", invFlags)
	if len(allVars) > 0 {
		extraVarsJSON, _ := json.Marshal(allVars)
		env = append(env, "GOMME_EXTRA_VARS="+string(extraVarsJSON))
		scriptParts = append(scriptParts, "printf '%s' \"$GOMME_EXTRA_VARS\" > /tmp/extra_vars.json")
		ansibleCmd += " -e '@/tmp/extra_vars.json'"
	}
	if limit != "" {
		ansibleCmd += " --limit " + limit
	}
	ansibleCmd += " " + playbookPath
	scriptParts = append(scriptParts, ansibleCmd)

	script := strings.Join(scriptParts, " && ")

	cfg := docker.ContainerConfig{
		Image:       dockerImage,
		Cmd:         []string{"sh", "-c", script},
		Env:         env,
		VolumesFrom: []string{"gomme-app"},
		WorkingDir:  "/app/repo",
		AutoRemove:  false,
	}

	containerID, err = h.Docker.CreateContainer(cfg)
	if err != nil {
		return "", "", fmt.Errorf("création conteneur: %w", err)
	}

	if err = h.Docker.StartContainer(containerID); err != nil {
		h.Docker.RemoveContainer(containerID)
		return "", containerID, fmt.Errorf("démarrage conteneur: %w", err)
	}

	// Sauvegarder containerID immédiatement pour que le streaming SSE puisse s'y connecter
	if runID > 0 {
		h.DB.Model(&models.PlaybookRun{ID: runID}).Update("container_id", containerID)
	}

	_, logCancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer logCancel()

	var buf strings.Builder
	h.Docker.StreamLogs(containerID, func(line string) {
		buf.WriteString(line)
	})

	exitCode, waitErr := h.Docker.WaitContainer(containerID)
	h.Docker.RemoveContainer(containerID)

	output = maskSecrets(buf.String(), secretValues)
	if waitErr != nil {
		return output, containerID, fmt.Errorf("attente conteneur: %w", waitErr)
	}
	if exitCode != 0 {
		return output, containerID, fmt.Errorf("ansible-playbook a retourné le code %d", exitCode)
	}
	return output, containerID, nil
}

// buildInventoryContent génère le contenu INI d'un inventaire.
// Si groupFilter est non vide, seuls les hôtes appartenant à ce groupe sont inclus.
func (h *Handler) buildInventoryContent(inventoryID uint, groupFilter string) (string, error) {
	if inventoryID == 0 {
		return "[all]\nlocalhost ansible_connection=local\n", nil
	}

	var hosts []models.Host
	h.DB.Preload("Groups").Where("inventory_id = ?", inventoryID).Find(&hosts)

	// Si filtrage par groupe : ne garder que les hôtes de ce groupe
	if groupFilter != "" {
		var filtered []models.Host
		for _, host := range hosts {
			for _, g := range host.Groups {
				if g.Name == groupFilter {
					filtered = append(filtered, host)
					break
				}
			}
		}
		hosts = filtered
	}

	var buf strings.Builder
	groupHosts := map[string][]string{}

	for _, host := range hosts {
		line := host.Name
		if host.IP != "" {
			line += " ansible_host=" + host.IP
		}
		// Injecter les variables de l'hôte inline (format key=value ou key: value)
		for _, varLine := range strings.Split(host.Vars, "\n") {
			varLine = strings.TrimSpace(varLine)
			if varLine == "" || strings.HasPrefix(varLine, "#") {
				continue
			}
			if strings.Contains(varLine, "=") {
				line += " " + varLine
			} else if idx := strings.Index(varLine, ": "); idx != -1 {
				line += " " + varLine[:idx] + "=" + varLine[idx+2:]
			} else if idx := strings.Index(varLine, ":"); idx != -1 {
				line += " " + varLine[:idx] + "=" + strings.TrimSpace(varLine[idx+1:])
			}
		}
		buf.WriteString(line + "\n")
		if groupFilter == "" {
			for _, g := range host.Groups {
				groupHosts[g.Name] = append(groupHosts[g.Name], host.Name)
			}
		} else {
			groupHosts[groupFilter] = append(groupHosts[groupFilter], host.Name)
		}
	}
	for grp, members := range groupHosts {
		buf.WriteString("\n[" + grp + "]\n")
		for _, m := range members {
			buf.WriteString(m + "\n")
		}
	}

	// Injecter les variables globales de l'inventaire dans [all:vars]
	var invVars []models.InventoryVar
	h.DB.Where("inventory_id = ?", inventoryID).Find(&invVars)
	if len(invVars) > 0 {
		buf.WriteString("\n[all:vars]\n")
		for _, v := range invVars {
			buf.WriteString(v.Key + "=" + v.Value + "\n")
		}
	}

	return buf.String(), nil
}

func (h *Handler) PlaybookInventoryAdd(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var pb models.Playbook
	if err := h.DB.First(&pb, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/playbooks?error=Playbook+introuvable")
	}
	if pb.OrganizationID != nil && !h.checkOrgAccess(user, *pb.OrganizationID, "update_playbook") {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?error=Accès+refusé", id))
	}

	invIDStr := c.FormValue("inventory_id")
	invID, err := strconv.ParseUint(invIDStr, 10, 64)
	if err != nil || invID == 0 {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?tab=inventories&error=Inventaire+invalide", id))
	}

	// Vérifier que l'inventaire existe
	var inv models.Inventory
	if h.DB.First(&inv, invID).Error != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?tab=inventories&error=Inventaire+introuvable", id))
	}

	link := models.PlaybookInventoryLink{
		PlaybookID:  uint(id),
		InventoryID: uint(invID),
		GroupFilter: strings.TrimSpace(c.FormValue("group_filter")),
	}
	h.DB.Create(&link)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?tab=inventories&success=Inventaire+ajouté", id))
}

func (h *Handler) PlaybookInventoryDelete(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	lid, _ := strconv.ParseUint(c.Param("lid"), 10, 64)

	var pb models.Playbook
	if err := h.DB.First(&pb, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/playbooks?error=Playbook+introuvable")
	}
	if pb.OrganizationID != nil && !h.checkOrgAccess(user, *pb.OrganizationID, "update_playbook") {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?error=Accès+refusé", id))
	}

	h.DB.Where("id = ? AND playbook_id = ?", lid, id).Delete(&models.PlaybookInventoryLink{})
	return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?tab=inventories&success=Inventaire+retiré", id))
}

func (h *Handler) PlaybookVarsList(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	data := PlaybookVarsData{
		User:    user,
		Success: c.QueryParam("success"),
		Error:   c.QueryParam("error"),
	}
	if err := h.DB.Preload("Repository").First(&data.Playbook, id).Error; err != nil {
		return echo.ErrNotFound
	}
	h.DB.Where("playbook_id = ?", id).Find(&data.Vars)
	return c.Render(http.StatusOK, "playbooks/vars", data)
}

func (h *Handler) PlaybookVarsSave(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var pb models.Playbook
	if err := h.DB.First(&pb, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/playbooks?error=Playbook+introuvable")
	}
	if pb.OrganizationID != nil && !h.checkOrgAccess(user, *pb.OrganizationID, "update_playbook") {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/vars?error=Accès+refusé", id))
	}

	params, _ := c.FormParams()
	keys := params["key"]
	values := params["value"]

	h.DB.Where("playbook_id = ?", id).Delete(&models.PlaybookVar{})
	for i, key := range keys {
		if key == "" {
			continue
		}
		val := ""
		if i < len(values) {
			val = values[i]
		}
		h.DB.Create(&models.PlaybookVar{
			PlaybookID: uint(id),
			Key:        key,
			Value:      val,
			Encrypted:  false,
		})
	}
	return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?tab=vars&success=Variables+sauvegardées", id))
}

func (h *Handler) PlaybookCredentialsSave(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var pb models.Playbook
	if err := h.DB.First(&pb, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/playbooks?error=Playbook+introuvable")
	}
	if pb.OrganizationID != nil && !h.checkOrgAccess(user, *pb.OrganizationID, "update_playbook") {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?error=Accès+refusé", id))
	}

	params, _ := c.FormParams()
	credIDStrs := params["credential_id"]

	var creds []models.Credential
	for _, cidStr := range credIDStrs {
		if cid, err := strconv.ParseUint(cidStr, 10, 64); err == nil && cid > 0 {
			creds = append(creds, models.Credential{ID: uint(cid)})
		}
	}

	h.DB.Model(&pb).Association("Credentials").Replace(creds)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?tab=credentials&success=Identifiants+mis+à+jour", id))
}

func (h *Handler) PlaybookSurvey(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	data := PlaybookSurveyData{User: user, Success: c.QueryParam("success")}
	if err := h.DB.Preload("Repository").First(&data.Playbook, id).Error; err != nil {
		return echo.ErrNotFound
	}
	h.DB.Where("playbook_id = ?", id).Order("sort_order").Find(&data.SurveyFields)
	return c.Render(http.StatusOK, "playbooks/survey", data)
}

func (h *Handler) PlaybookSurveySave(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var pb models.Playbook
	if err := h.DB.First(&pb, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/playbooks?error=Playbook+introuvable")
	}
	if pb.OrganizationID != nil && !h.checkOrgAccess(user, *pb.OrganizationID, "update_playbook") {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/survey?error=Accès+refusé", id))
	}

	params, _ := c.FormParams()
	labels := params["label"]
	varNames := params["var_name"]
	types := params["type"]
	options := params["options"]
	defaults := params["default"]
	requiredFlags := params["required"]

	h.DB.Where("playbook_id = ?", id).Delete(&models.SurveyField{})
	for i, label := range labels {
		if label == "" {
			continue
		}
		varName, fieldType, opts, def := "", "text", "", ""
		if i < len(varNames) {
			varName = varNames[i]
		}
		if i < len(types) && types[i] != "" {
			fieldType = types[i]
		}
		if i < len(options) {
			opts = options[i]
		}
		if i < len(defaults) {
			def = defaults[i]
		}
		isReq := i < len(requiredFlags) && requiredFlags[i] == "on"
		h.DB.Create(&models.SurveyField{
			PlaybookID: uint(id),
			Label:      label,
			VarName:    varName,
			Type:       fieldType,
			Options:    opts,
			Default:    def,
			Required:   isReq,
			SortOrder:  i,
		})
	}
	return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?tab=survey&success=Formulaire+sauvegardé", id))
}

func (h *Handler) PlaybookEdit(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	data := PlaybookEditData{
		User:    user,
		Success: c.QueryParam("success"),
		Error:   c.QueryParam("error"),
		Tab:     c.QueryParam("tab"),
	}
	if data.Tab == "" {
		data.Tab = "general"
	}
	if err := h.DB.Preload("Repository").Preload("Credentials").Preload("Inventories.Inventory").First(&data.Playbook, id).Error; err != nil {
		return echo.ErrNotFound
	}
	data.LinkedInventories = data.Playbook.Inventories
	h.DB.Where("playbook_id = ?", id).Find(&data.Vars)
	h.DB.Where("playbook_id = ?", id).Order("sort_order").Find(&data.SurveyFields)

	data.LinkedCredentialIDs = make(map[uint]bool)
	for _, cr := range data.Playbook.Credentials {
		data.LinkedCredentialIDs[cr.ID] = true
	}
	orgIDs := h.accessibleOrgIDs(user)
	if user.IsAdmin {
		h.DB.Preload("Organization").Preload("Fields").Find(&data.AvailableCredentials)
	} else if len(orgIDs) == 0 {
		h.DB.Preload("Organization").Preload("Fields").Where("organization_id IS NULL AND user_id = ?", user.ID).Find(&data.AvailableCredentials)
	} else {
		h.DB.Preload("Organization").Preload("Fields").
			Where("(organization_id IS NULL AND user_id = ?) OR (organization_id IN ?)", user.ID, orgIDs).
			Find(&data.AvailableCredentials)
	}

	data.Organizations = h.userOrgs(user.ID)
	h.DB.Find(&data.Repositories)
	var allInvs []models.Inventory
	h.DB.Find(&allInvs)
	for _, inv := range allInvs {
		if inv.OrganizationID == nil || user.IsAdmin || h.checkOrgAccess(user, *inv.OrganizationID, "update_inventory") {
			data.Inventories = append(data.Inventories, inv)
		}
	}
	if data.Playbook.OrganizationID != nil {
		data.OrganizationID = *data.Playbook.OrganizationID
	}
	if data.Playbook.Repository.LocalPath != "" {
		filepath.Walk(data.Playbook.Repository.LocalPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			if strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
				rel, _ := filepath.Rel(data.Playbook.Repository.LocalPath, path)
				data.RepoFiles = append(data.RepoFiles, rel)
			}
			return nil
		})
	}
	h.DB.Order("name").Find(&data.DockerImages)
	h.DB.Where("playbook_id = ?", id).Order("created_at desc").Find(&data.ScheduledTasks)
	return c.Render(http.StatusOK, "playbooks/edit", data)
}

func (h *Handler) RunDetail(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	data := RunDetailData{User: user}
	if err := h.DB.
		Preload("Playbook.Vars").
		Preload("Playbook.SurveyFields").
		Preload("Playbook.Credentials").
		Preload("Inventories.Inventory").
		First(&data.Run, id).Error; err != nil {
		return echo.ErrNotFound
	}
	return c.Render(http.StatusOK, "playbooks/run_detail", data)
}

func (h *Handler) RunOutput(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var run models.PlaybookRun
	if err := h.DB.Select("output", "status").First(&run, id).Error; err != nil {
		return echo.ErrNotFound
	}
	return c.JSON(http.StatusOK, map[string]string{
		"output": run.Output,
		"status": run.Status,
	})
}

func (h *Handler) PlaybookUpdateOrg(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var pb models.Playbook
	if err := h.DB.First(&pb, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/playbooks?error=Playbook+introuvable")
	}
	if pb.OrganizationID != nil && !h.checkOrgAccess(user, *pb.OrganizationID, "update_playbook") {
		return c.Redirect(http.StatusFound, "/playbooks?error=Accès+refusé")
	}
	if orgIDStr := c.FormValue("organization_id"); orgIDStr != "" {
		if orgID, err := strconv.ParseUint(orgIDStr, 10, 64); err == nil && orgID > 0 {
			if !h.checkOrgAccess(user, uint(orgID), "update_playbook") {
				return c.Redirect(http.StatusFound, "/playbooks?error=Accès+refusé")
			}
			oid := uint(orgID)
			pb.OrganizationID = &oid
		} else {
			pb.OrganizationID = nil
		}
	} else {
		pb.OrganizationID = nil
	}
	h.DB.Save(&pb)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/playbooks/%d/edit?success=Organisation+mise+à+jour", id))
}

// RunLogsStream envoie les logs d'un run en temps réel via Server-Sent Events.
func (h *Handler) RunLogsStream(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var run models.PlaybookRun
	if err := h.DB.Select("id", "status", "container_id", "output").First(&run, id).Error; err != nil {
		return echo.ErrNotFound
	}

	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flush := func() {
		if f, ok := w.Writer.(http.Flusher); ok {
			f.Flush()
		}
	}
	sendEvent := func(event, data string) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b))
		flush()
	}

	// Run déjà terminé : envoyer l'output stocké et fermer
	if run.Status != "running" && run.Status != "pending" {
		sendEvent("output", run.Output)
		sendEvent("done", run.Status)
		return nil
	}

	// Attendre que le containerID soit disponible (le conteneur peut encore démarrer)
	containerID := run.ContainerID
	for i := 0; containerID == "" && i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		var cur models.PlaybookRun
		h.DB.Select("container_id", "status", "output").First(&cur, id)
		containerID = cur.ContainerID
		if cur.Status != "running" && cur.Status != "pending" {
			sendEvent("output", cur.Output)
			sendEvent("done", cur.Status)
			return nil
		}
	}
	if containerID == "" {
		sendEvent("error", "Container non disponible")
		return nil
	}

	// Streamer les logs depuis Docker
	reader, err := h.Docker.LogsReader(containerID, true)
	if err != nil {
		sendEvent("error", err.Error())
		return nil
	}
	defer reader.Close()

	hdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(reader, hdr); err != nil {
			break
		}
		size := binary.BigEndian.Uint32(hdr[4:8])
		if size == 0 {
			continue
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(reader, data); err != nil {
			break
		}
		sendEvent("chunk", string(data))
	}

	// Statut final
	var finalRun models.PlaybookRun
	h.DB.Select("status").First(&finalRun, id)
	sendEvent("done", finalRun.Status)
	return nil
}

// maskSecrets remplace chaque valeur secrète par "****" dans l'output Ansible.
func maskSecrets(output string, secrets []string) string {
	for _, s := range secrets {
		if s != "" {
			output = strings.ReplaceAll(output, s, "****")
		}
	}
	return output
}
