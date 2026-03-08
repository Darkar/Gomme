package handlers

import (
	"encoding/json"
	"fmt"
	"gomme/crypto"
	"gomme/inventory"
	"gomme/models"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// InventoryView embarque un Inventory avec les champs de config décodés (sans mot de passe).
type InventoryView struct {
	models.Inventory
	ParsedAuthMode string
	// API
	ParsedURL        string
	ParsedUser       string
	ParsedNode       string
	ParsedInsecure   bool
	ParsedFilterTags string
	ParsedAPITokenID string
	// SSH
	ParsedSSHHost string
	ParsedSSHPort string
	ParsedSSHUser string
	// Permissions
	CanEdit   bool
	CanDelete bool
}

type InventoryListData struct {
	User          *models.User
	Inventories   []InventoryView
	Organizations []models.Organization
	HostCounts    map[uint]int64
	Success       string
	Error         string
}

func parseStoredConfig(configJSON string) inventory.StoredProxmoxConfig {
	var cfg inventory.StoredProxmoxConfig
	json.Unmarshal([]byte(configJSON), &cfg) //nolint — zero value si JSON invalide
	return cfg
}

func (h *Handler) InventoryRedirect(c echo.Context) error {
	return c.Redirect(http.StatusFound, "/inventory/list")
}

func (h *Handler) InventoryList(c echo.Context) error {
	user := c.Get("user").(*models.User)
	data := InventoryListData{
		User:       user,
		HostCounts: map[uint]int64{},
		Success:    c.QueryParam("success"),
		Error:      c.QueryParam("error"),
	}
	data.Organizations = h.userOrgs(user.ID)
	var invs []models.Inventory
	h.DB.Preload("Organization").Find(&invs)
	for _, inv := range invs {
		var count int64
		h.DB.Model(&models.Host{}).Where("inventory_id = ?", inv.ID).Count(&count)
		data.HostCounts[inv.ID] = count

		cfg := parseStoredConfig(inv.Config)
		canEdit := inv.OrganizationID == nil || h.checkOrgAccess(user, *inv.OrganizationID, "update_inventory")
		canDelete := inv.OrganizationID == nil || h.checkOrgAccess(user, *inv.OrganizationID, "delete_inventory")
		data.Inventories = append(data.Inventories, InventoryView{
			Inventory:        inv,
			ParsedAuthMode:   cfg.AuthMode,
			ParsedURL:        cfg.URL,
			ParsedUser:       cfg.User,
			ParsedNode:       cfg.Node,
			ParsedInsecure:   cfg.Insecure,
			ParsedFilterTags: cfg.FilterTags,
			ParsedAPITokenID: cfg.APITokenID,
			ParsedSSHHost:    cfg.SSHHost,
			ParsedSSHPort:    cfg.SSHPort,
			ParsedSSHUser:    cfg.SSHUser,
			CanEdit:          canEdit,
			CanDelete:        canDelete,
		})
	}
	return c.Render(http.StatusOK, "inventory/list", data)
}

func (h *Handler) InventoryCreate(c echo.Context) error {
	name := c.FormValue("name")
	source := c.FormValue("source")
	if name == "" || source == "" {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Nom+et+source+requis")
	}
	configJSON, err := h.buildInventoryConfig(source, c, "")
	if err != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/list?error=%s", err.Error()))
	}
	user := c.Get("user").(*models.User)
	inv := models.Inventory{Name: name, Source: source, Config: configJSON, UserID: user.ID}
	if orgIDStr := c.FormValue("organization_id"); orgIDStr != "" {
		if orgID, err := strconv.ParseUint(orgIDStr, 10, 64); err == nil && orgID > 0 {
			if !h.checkOrgAccess(user, uint(orgID), "create_inventory") {
				return c.Redirect(http.StatusFound, "/inventory/list?error=Accès+refusé")
			}
			id := uint(orgID)
			inv.OrganizationID = &id
		}
	}
	h.DB.Create(&inv)
	return c.Redirect(http.StatusFound, "/inventory/list?success=Inventaire+créé")
}

func (h *Handler) InventoryUpdate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var inv models.Inventory
	if err := h.DB.First(&inv, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Inventaire+introuvable")
	}
	if inv.OrganizationID != nil && !h.checkOrgAccess(user, *inv.OrganizationID, "update_inventory") {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Accès+refusé")
	}
	source := c.FormValue("source")
	configJSON, err := h.buildInventoryConfig(source, c, inv.Config)
	if err != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=source&error=%s", id, err.Error()))
	}
	inv.Name = c.FormValue("name")
	inv.Source = source
	inv.Config = configJSON
	if orgIDStr := c.FormValue("organization_id"); orgIDStr != "" {
		if orgID, err := strconv.ParseUint(orgIDStr, 10, 64); err == nil && orgID > 0 {
			oid := uint(orgID)
			inv.OrganizationID = &oid
		} else {
			inv.OrganizationID = nil
		}
	} else {
		inv.OrganizationID = nil
	}
	h.DB.Save(&inv)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=source&success=Inventaire+mis+à+jour", id))
}

// buildInventoryConfig construit le JSON de config à partir des champs de formulaire.
// existingConfig est utilisé pour conserver les mots de passe chiffrés existants si les champs sont vides.
func (h *Handler) buildInventoryConfig(sourceType string, c echo.Context, existingConfig string) (string, error) {
	switch sourceType {
	case "manual":
		return "", nil
	case "proxmox":
		existing := parseStoredConfig(existingConfig)
		authMode := c.FormValue("proxmox_auth_mode")
		if authMode == "" {
			authMode = "api"
		}
		cfg := inventory.StoredProxmoxConfig{
			AuthMode:          authMode,
			PasswordEnc:       existing.PasswordEnc,
			SSHPasswordEnc:    existing.SSHPasswordEnc,
			APITokenSecretEnc: existing.APITokenSecretEnc,
		}
		cfg.FilterTags = c.FormValue("proxmox_filter_tags")
		if authMode == "api" {
			cfg.URL = c.FormValue("proxmox_url")
			cfg.Node = c.FormValue("proxmox_node")
			cfg.Insecure = c.FormValue("proxmox_insecure") == "on"
			apiSubMode := c.FormValue("proxmox_api_sub_mode")
			if apiSubMode == "token" {
				cfg.APITokenID = c.FormValue("proxmox_api_token_id")
				if secret := c.FormValue("proxmox_api_token_secret"); secret != "" {
					enc, err := crypto.Encrypt(h.Config.SecretKey, secret)
					if err != nil {
						return "", fmt.Errorf("chiffrement secret token API: %w", err)
					}
					cfg.APITokenSecretEnc = enc
				}
			} else {
				cfg.User = c.FormValue("proxmox_user")
				if pwd := c.FormValue("proxmox_password"); pwd != "" {
					enc, err := crypto.Encrypt(h.Config.SecretKey, pwd)
					if err != nil {
						return "", fmt.Errorf("chiffrement mot de passe API: %w", err)
					}
					cfg.PasswordEnc = enc
				}
			}
		} else {
			cfg.SSHHost = c.FormValue("proxmox_ssh_host")
			cfg.SSHPort = c.FormValue("proxmox_ssh_port")
			cfg.SSHUser = c.FormValue("proxmox_ssh_user")
			if pwd := c.FormValue("proxmox_ssh_password"); pwd != "" {
				enc, err := crypto.Encrypt(h.Config.SecretKey, pwd)
				if err != nil {
					return "", fmt.Errorf("chiffrement mot de passe SSH: %w", err)
				}
				cfg.SSHPasswordEnc = enc
			}
		}
		b, _ := json.Marshal(cfg)
		return string(b), nil
	default:
		return "", nil
	}
}

func (h *Handler) InventoryDelete(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var inv models.Inventory
	if err := h.DB.First(&inv, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Inventaire+introuvable")
	}
	if inv.OrganizationID != nil && !h.checkOrgAccess(user, *inv.OrganizationID, "delete_inventory") {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Accès+refusé")
	}
	var hostIDs []uint
	h.DB.Model(&models.Host{}).Where("inventory_id = ?", id).Pluck("id", &hostIDs)
	if len(hostIDs) > 0 {
		h.DB.Exec("DELETE FROM host_groups WHERE host_id IN ?", hostIDs)
	}
	h.DB.Where("inventory_id = ?", id).Delete(&models.Host{})
	h.DB.Where("inventory_id = ?", id).Delete(&models.Group{})
	h.DB.Where("inventory_id = ?", id).Delete(&models.InventoryVar{})
	h.DB.Delete(&models.Inventory{}, id)
	return c.Redirect(http.StatusFound, "/inventory/list?success=Inventaire+supprimé")
}

func (h *Handler) InventorySync(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var inv models.Inventory
	if err := h.DB.First(&inv, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Inventaire+introuvable")
	}

	if inv.Source == "manual" {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Les+inventaires+manuels+ne+se+synchronisent+pas")
	}

	src, err := inventory.GetSource(inv.Source, inv.Config, h.Config.SecretKey)
	if err != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/list?error=%s", err.Error()))
	}

	hosts, groups, err := src.Sync()
	if err != nil {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/list?error=Sync+échouée+%%3A+%s", err.Error()))
	}

	// Clean up host_groups join table before deleting hosts
	var existingHostIDs []uint
	h.DB.Model(&models.Host{}).Where("inventory_id = ?", inv.ID).Pluck("id", &existingHostIDs)
	if len(existingHostIDs) > 0 {
		h.DB.Exec("DELETE FROM host_groups WHERE host_id IN ?", existingHostIDs)
	}
	h.DB.Where("inventory_id = ?", inv.ID).Delete(&models.Host{})
	h.DB.Where("inventory_id = ?", inv.ID).Delete(&models.Group{})

	// Deduplicate groups by name
	groupMap := map[string]uint{}
	for _, g := range groups {
		if _, exists := groupMap[g.Name]; exists {
			continue
		}
		grp := models.Group{InventoryID: inv.ID, Name: g.Name, Description: g.Description}
		h.DB.Create(&grp)
		groupMap[g.Name] = grp.ID
	}

	for _, hd := range hosts {
		host := models.Host{
			InventoryID: inv.ID,
			Name:        hd.Name,
			IP:          hd.IP,
			Description: hd.Description,
		}
		h.DB.Create(&host)
		seen := map[uint]bool{}
		for _, gName := range hd.Groups {
			if gID, ok := groupMap[gName]; ok && !seen[gID] {
				h.DB.Exec("INSERT INTO host_groups (host_id, group_id) VALUES (?, ?)", host.ID, gID)
				seen[gID] = true
			}
		}
	}

	now := time.Now()
	h.DB.Model(&inv).Update("last_sync_at", &now)

	return c.Redirect(http.StatusFound, "/inventory/list?success=Inventaire+synchronisé")
}

// ── Détail inventaire (5 onglets) ───────────────────────────────────────────

type InventoryDetailData struct {
	User              *models.User
	Inventory         models.Inventory
	Hosts             []models.Host
	Groups            []models.Group
	GroupHostCounts   map[uint]int
	Vars              []models.InventoryVar
	Tab               string
	IsManual          bool
	ParsedSource      InventoryView
	Organizations     []models.Organization
	OrganizationIDVal uint // valeur déréférencée de Inventory.OrganizationID (0 si nil)
	Success           string
	Error             string
}

func (h *Handler) InventoryDetail(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var inv models.Inventory
	if err := h.DB.Preload("Organization").First(&inv, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Inventaire+introuvable")
	}

	tab := c.QueryParam("tab")
	if tab == "" {
		tab = "groups"
	}
	// Source tab uniquement pour les non-manuels
	if tab == "source" && inv.Source == "manual" {
		tab = "groups"
	}

	cfg := parseStoredConfig(inv.Config)
	parsedSource := InventoryView{
		Inventory:        inv,
		ParsedAuthMode:   cfg.AuthMode,
		ParsedURL:        cfg.URL,
		ParsedUser:       cfg.User,
		ParsedNode:       cfg.Node,
		ParsedInsecure:   cfg.Insecure,
		ParsedFilterTags: cfg.FilterTags,
		ParsedAPITokenID: cfg.APITokenID,
		ParsedSSHHost:    cfg.SSHHost,
		ParsedSSHPort:    cfg.SSHPort,
		ParsedSSHUser:    cfg.SSHUser,
	}

	var orgIDVal uint
	if inv.OrganizationID != nil {
		orgIDVal = *inv.OrganizationID
	}
	data := InventoryDetailData{
		User:              user,
		Inventory:         inv,
		GroupHostCounts:   map[uint]int{},
		Tab:               tab,
		IsManual:          inv.Source == "manual",
		ParsedSource:      parsedSource,
		Organizations:     h.userOrgs(user.ID),
		OrganizationIDVal: orgIDVal,
		Success:           c.QueryParam("success"),
		Error:             c.QueryParam("error"),
	}

	h.DB.Where("inventory_id = ?", inv.ID).Find(&data.Groups)
	h.DB.Where("inventory_id = ?", inv.ID).Preload("Groups").Find(&data.Hosts)
	for _, host := range data.Hosts {
		for _, g := range host.Groups {
			data.GroupHostCounts[g.ID]++
		}
	}
	h.DB.Where("inventory_id = ?", inv.ID).Find(&data.Vars)

	return c.Render(http.StatusOK, "inventory/detail", data)
}

// ── Groupes ──────────────────────────────────────────────────────────────────

func (h *Handler) InventoryGroupCreate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var inv models.Inventory
	if err := h.DB.First(&inv, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Inventaire+introuvable")
	}
	if inv.Source != "manual" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=groups&error=Cet+inventaire+n'est+pas+manuel", id))
	}
	if inv.OrganizationID != nil && !h.checkOrgAccess(user, *inv.OrganizationID, "update_inventory") {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=groups&error=Accès+refusé", id))
	}
	name := c.FormValue("name")
	if name == "" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=groups&error=Nom+requis", id))
	}
	grp := models.Group{InventoryID: uint(id), Name: name, Description: c.FormValue("description")}
	h.DB.Create(&grp)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=groups&success=Groupe+ajouté", id))
}

func (h *Handler) InventoryGroupDelete(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	gid, _ := strconv.ParseUint(c.Param("gid"), 10, 64)
	var inv models.Inventory
	if err := h.DB.First(&inv, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Inventaire+introuvable")
	}
	if inv.OrganizationID != nil && !h.checkOrgAccess(user, *inv.OrganizationID, "update_inventory") {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=groups&error=Accès+refusé", id))
	}
	var grp models.Group
	if err := h.DB.First(&grp, gid).Error; err != nil || grp.InventoryID != uint(id) {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=groups&error=Groupe+introuvable", id))
	}
	h.DB.Exec("DELETE FROM host_groups WHERE group_id = ?", grp.ID)
	h.DB.Delete(&grp)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=groups&success=Groupe+supprimé", id))
}

// ── Hôtes ────────────────────────────────────────────────────────────────────

func (h *Handler) InventoryHostCreate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var inv models.Inventory
	if err := h.DB.First(&inv, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Inventaire+introuvable")
	}
	if inv.Source != "manual" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=hosts&error=Cet+inventaire+n'est+pas+manuel", id))
	}
	if inv.OrganizationID != nil && !h.checkOrgAccess(user, *inv.OrganizationID, "update_inventory") {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=hosts&error=Accès+refusé", id))
	}
	name := c.FormValue("name")
	if name == "" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=hosts&error=Nom+requis", id))
	}
	host := models.Host{
		InventoryID: uint(id),
		Name:        name,
		IP:          c.FormValue("ip"),
		Description: c.FormValue("description"),
		Vars:        c.FormValue("vars"),
	}
	h.DB.Create(&host)
	params, _ := c.FormParams()
	for _, gIDStr := range params["group_id"] {
		if gID, err := strconv.ParseUint(gIDStr, 10, 64); err == nil {
			h.DB.Exec("INSERT INTO host_groups (host_id, group_id) VALUES (?, ?)", host.ID, gID)
		}
	}
	return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=hosts&success=Hôte+ajouté", id))
}

func (h *Handler) InventoryHostUpdate(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	hid, _ := strconv.ParseUint(c.Param("hid"), 10, 64)
	var inv models.Inventory
	if err := h.DB.First(&inv, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Inventaire+introuvable")
	}
	if inv.OrganizationID != nil && !h.checkOrgAccess(user, *inv.OrganizationID, "update_inventory") {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=hosts&error=Accès+refusé", id))
	}
	var host models.Host
	if err := h.DB.First(&host, hid).Error; err != nil || host.InventoryID != uint(id) {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=hosts&error=Hôte+introuvable", id))
	}
	name := c.FormValue("name")
	if name == "" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=hosts&error=Nom+requis", id))
	}
	host.Name = name
	host.IP = c.FormValue("ip")
	host.Description = c.FormValue("description")
	host.Vars = c.FormValue("vars")
	h.DB.Save(&host)
	h.DB.Exec("DELETE FROM host_groups WHERE host_id = ?", host.ID)
	params, _ := c.FormParams()
	for _, gIDStr := range params["group_id"] {
		if gID, err := strconv.ParseUint(gIDStr, 10, 64); err == nil {
			h.DB.Exec("INSERT INTO host_groups (host_id, group_id) VALUES (?, ?)", host.ID, gID)
		}
	}
	return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=hosts&success=Hôte+mis+à+jour", id))
}

func (h *Handler) InventoryHostDelete(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	hid, _ := strconv.ParseUint(c.Param("hid"), 10, 64)
	var inv models.Inventory
	if err := h.DB.First(&inv, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Inventaire+introuvable")
	}
	if inv.OrganizationID != nil && !h.checkOrgAccess(user, *inv.OrganizationID, "update_inventory") {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=hosts&error=Accès+refusé", id))
	}
	var host models.Host
	if err := h.DB.First(&host, hid).Error; err != nil || host.InventoryID != uint(id) {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=hosts&error=Hôte+introuvable", id))
	}
	h.DB.Exec("DELETE FROM host_groups WHERE host_id = ?", host.ID)
	h.DB.Delete(&host)
	return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=hosts&success=Hôte+supprimé", id))
}

func (h *Handler) InventoryHostAPI(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	hid, _ := strconv.ParseUint(c.Param("hid"), 10, 64)
	var host models.Host
	if err := h.DB.Preload("Groups").First(&host, hid).Error; err != nil || host.InventoryID != uint(id) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "introuvable"})
	}
	groupIDs := make([]uint, len(host.Groups))
	for i, g := range host.Groups {
		groupIDs[i] = g.ID
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":          host.ID,
		"name":        host.Name,
		"ip":          host.IP,
		"description": host.Description,
		"vars":        host.Vars,
		"group_ids":   groupIDs,
	})
}

// ── Graphe par inventaire ────────────────────────────────────────────────────

func (h *Handler) InventoryGraphDetail(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var inv models.Inventory
	if err := h.DB.First(&inv, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Inventaire+introuvable")
	}
	return c.Render(http.StatusOK, "inventory/detail", InventoryDetailData{
		User:      user,
		Inventory: inv,
		Tab:       "graph",
		IsManual:  inv.Source == "manual",
	})
}

func (h *Handler) InventoryGraphDetailAPI(c echo.Context) error {
	type Node struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Type  string `json:"type"`
		IP    string `json:"ip,omitempty"`
	}
	type Link struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var hosts []models.Host
	var groups []models.Group
	h.DB.Preload("Groups").Where("inventory_id = ?", id).Find(&hosts)
	h.DB.Where("inventory_id = ?", id).Find(&groups)

	var nodes []Node
	var links []Link

	for _, g := range groups {
		nodes = append(nodes, Node{ID: fmt.Sprintf("g_%d", g.ID), Label: g.Name, Type: "group"})
	}
	for _, host := range hosts {
		hID := fmt.Sprintf("h_%d", host.ID)
		nodes = append(nodes, Node{ID: hID, Label: host.Name, Type: "host", IP: host.IP})
		for _, g := range host.Groups {
			links = append(links, Link{Source: hID, Target: fmt.Sprintf("g_%d", g.ID)})
		}
	}

	if nodes == nil {
		nodes = []Node{}
	}
	if links == nil {
		links = []Link{}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"nodes": nodes, "links": links})
}

// ── Variables d'inventaire ──────────────────────────────────────────────────

func (h *Handler) InventoryVarsSave(c echo.Context) error {
	user := c.Get("user").(*models.User)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var inv models.Inventory
	if err := h.DB.First(&inv, id).Error; err != nil {
		return c.Redirect(http.StatusFound, "/inventory/list?error=Inventaire+introuvable")
	}
	if inv.OrganizationID != nil && !h.checkOrgAccess(user, *inv.OrganizationID, "update_inventory") {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=vars&error=Accès+refusé", id))
	}
	params, _ := c.FormParams()
	keys := params["key"]
	values := params["value"]
	h.DB.Where("inventory_id = ?", id).Delete(&models.InventoryVar{})
	for i, key := range keys {
		if key == "" {
			continue
		}
		val := ""
		if i < len(values) {
			val = values[i]
		}
		h.DB.Create(&models.InventoryVar{InventoryID: uint(id), Key: key, Value: val})
	}
	return c.Redirect(http.StatusFound, fmt.Sprintf("/inventory/%d?tab=vars&success=Variables+sauvegardées", id))
}

func (h *Handler) InventoryVarsAPI(c echo.Context) error {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var vars []models.InventoryVar
	h.DB.Where("inventory_id = ?", id).Find(&vars)
	if vars == nil {
		vars = []models.InventoryVar{}
	}
	return c.JSON(http.StatusOK, vars)
}
