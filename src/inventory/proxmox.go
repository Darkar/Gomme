package inventory

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/crypto/ssh"
)

type ProxmoxConfig struct {
	AuthMode string // "api" ou "ssh"

	// API
	URL      string
	User     string
	Password string
	Node     string
	Insecure bool

	// Token API (alternative à user/password)
	APITokenID     string
	APITokenSecret string

	// SSH
	SSHHost     string
	SSHPort     string
	SSHUser     string
	SSHPassword string

	// Filtre par tags (vide = tout inclure)
	FilterTags []string
}

type ProxmoxSource struct {
	Config ProxmoxConfig
}

type proxmoxVM struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Tags   string `json:"tags"` // séparés par ";" dans l'API Proxmox
}

func (p *ProxmoxSource) Sync() ([]HostData, []GroupData, error) {
	if p.Config.AuthMode == "ssh" {
		return p.syncViaSSH()
	}
	return p.syncViaAPI()
}

// ── API ──────────────────────────────────────────────────────────────────────

// setAuth applique les headers d'authentification sur une requête.
// Token API si configuré, sinon cookie/CSRF.
func (p *ProxmoxSource) setAuth(req *http.Request, ticket, csrf string) {
	if p.Config.APITokenID != "" {
		req.Header.Set("Authorization", "PVEAPIToken="+p.Config.APITokenID+"="+p.Config.APITokenSecret)
	} else {
		req.Header.Set("Cookie", "PVEAuthCookie="+ticket)
		req.Header.Set("CSRFPreventionToken", csrf)
	}
}

func (p *ProxmoxSource) syncViaAPI() ([]HostData, []GroupData, error) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: p.Config.Insecure},
		},
	}

	var ticket, csrf string
	if p.Config.APITokenID == "" {
		var err error
		ticket, csrf, err = p.getTicket(client)
		if err != nil {
			return nil, nil, fmt.Errorf("authentification proxmox: %w", err)
		}
	}

	// Si FilterTags est renseigné, seuls ces tags deviennent des groupes
	allowedTags := map[string]bool{}
	for _, t := range p.Config.FilterTags {
		allowedTags[strings.ToLower(t)] = true
	}
	hasFilter := len(allowedTags) > 0

	groupSet := map[string]GroupData{}
	var hosts []HostData

	for _, vmType := range []string{"qemu", "lxc"} {
		vms, err := p.fetchVMs(client, ticket, csrf, vmType)
		if err != nil {
			return nil, nil, fmt.Errorf("récupération %s: %w", vmType, err)
		}
		for _, vm := range vms {
			// Récupérer les tags depuis la config individuelle de la VM
			rawTags := vm.Tags
			if rawTags == "" {
				rawTags = p.fetchVMTags(client, ticket, csrf, vmType, vm.VMID)
			}
			// Parser les tags Proxmox (séparés par ";" ou "," selon la version)
			var vmTags []string
			for _, t := range strings.FieldsFunc(rawTags, func(r rune) bool { return r == ';' || r == ',' }) {
				t = strings.TrimSpace(t)
				if t != "" {
					vmTags = append(vmTags, t)
				}
			}

			// Un groupe par tag ; si filtre actif, seuls les tags autorisés deviennent des groupes
			var hostGroups []string
			for _, t := range vmTags {
				if hasFilter && !allowedTags[strings.ToLower(t)] {
					continue
				}
				hostGroups = append(hostGroups, t)
				if _, exists := groupSet[t]; !exists {
					groupSet[t] = GroupData{Name: t, Description: "Tag Proxmox"}
				}
			}

			// Tous les hôtes sont ajoutés, qu'ils aient des tags ou non
			hosts = append(hosts, HostData{Name: vm.Name, Groups: hostGroups})
		}
	}

	groups := make([]GroupData, 0, len(groupSet))
	for _, g := range groupSet {
		groups = append(groups, g)
	}
	return hosts, groups, nil
}

func (p *ProxmoxSource) getTicket(client *http.Client) (ticket, csrf string, err error) {
	apiURL := fmt.Sprintf("%s/api2/json/access/ticket", p.Config.URL)

	form := url.Values{}
	form.Set("username", p.Config.User)
	form.Set("password", p.Config.Password)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("authentification refusée (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			Ticket              string `json:"ticket"`
			CSRFPreventionToken string `json:"CSRFPreventionToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("réponse inattendue: %w", err)
	}
	if result.Data.Ticket == "" {
		return "", "", fmt.Errorf("ticket vide — vérifiez les credentials")
	}
	return result.Data.Ticket, result.Data.CSRFPreventionToken, nil
}

func (p *ProxmoxSource) fetchVMs(client *http.Client, ticket, csrf, vmType string) ([]proxmoxVM, error) {
	apiURL := fmt.Sprintf("%s/api2/json/nodes/%s/%s", p.Config.URL, p.Config.Node, vmType)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	p.setAuth(req, ticket, csrf)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []proxmoxVM `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing réponse: %w", err)
	}
	return result.Data, nil
}

// fetchVMTags récupère les tags d'une VM/CT depuis son endpoint de config.
func (p *ProxmoxSource) fetchVMTags(client *http.Client, ticket, csrf, vmType string, vmid int) string {
	apiURL := fmt.Sprintf("%s/api2/json/nodes/%s/%s/%d/config", p.Config.URL, p.Config.Node, vmType, vmid)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return ""
	}
	p.setAuth(req, ticket, csrf)

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Data struct {
			Tags string `json:"tags"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return ""
	}
	return result.Data.Tags
}

// ── SSH ──────────────────────────────────────────────────────────────────────

func (p *ProxmoxSource) syncViaSSH() ([]HostData, []GroupData, error) {
	port := p.Config.SSHPort
	if port == "" {
		port = "22"
	}

	sshCfg := &ssh.ClientConfig{
		User: p.Config.SSHUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(p.Config.SSHPassword),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint — outil interne, sans vérification de clé hôte
	}

	client, err := ssh.Dial("tcp", p.Config.SSHHost+":"+port, sshCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("connexion SSH à %s:%s : %w", p.Config.SSHHost, port, err)
	}
	defer client.Close()

	var hosts []HostData

	if out, err := runSSH(client, "qm list"); err == nil {
		for _, name := range parseQMList(out) {
			hosts = append(hosts, HostData{Name: name})
		}
	}

	if out, err := runSSH(client, "pct list"); err == nil {
		for _, name := range parsePCTList(out) {
			hosts = append(hosts, HostData{Name: name})
		}
	}

	return hosts, nil, nil
}

func runSSH(client *ssh.Client, cmd string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out, err := sess.Output(cmd)
	return string(out), err
}

// parseQMList parse la sortie de `qm list` :
//
//	VMID NAME   STATUS  MEM(MB) BOOTDISK(GB) PID
//	 100 myvm   running    2048        32.00 1234
func parseQMList(output string) []string {
	var names []string
	for i, line := range strings.Split(output, "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		if fields := strings.Fields(line); len(fields) >= 2 {
			names = append(names, fields[1])
		}
	}
	return names
}

// parsePCTList parse la sortie de `pct list` :
//
//	CTID  Status  Lock  Hostname
//	 101  running       myct
//
// La colonne Lock peut être vide ; le hostname est toujours le dernier champ.
func parsePCTList(output string) []string {
	var names []string
	for i, line := range strings.Split(output, "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		if fields := strings.Fields(line); len(fields) >= 3 {
			names = append(names, fields[len(fields)-1])
		}
	}
	return names
}
