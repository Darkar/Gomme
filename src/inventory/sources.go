package inventory

import (
	"encoding/json"
	"fmt"
	"gomme/crypto"
	"strings"
)

type HostData struct {
	Name        string
	IP          string
	Description string
	Groups      []string
}

type GroupData struct {
	Name        string
	Description string
}

type Source interface {
	Sync() ([]HostData, []GroupData, error)
}

// StoredProxmoxConfig est le format JSON stocké en base (mots de passe chiffrés).
type StoredProxmoxConfig struct {
	AuthMode string `json:"auth_mode"` // "api" ou "ssh"

	// Champs API
	URL         string `json:"url,omitempty"`
	User        string `json:"user,omitempty"`
	PasswordEnc string `json:"password_enc,omitempty"`
	Node        string `json:"node,omitempty"`
	Insecure    bool   `json:"insecure,omitempty"`

	// Token API (alternative à user/password)
	APITokenID        string `json:"api_token_id,omitempty"`
	APITokenSecretEnc string `json:"api_token_secret_enc,omitempty"`

	// Champs SSH
	SSHHost        string `json:"ssh_host,omitempty"`
	SSHPort        string `json:"ssh_port,omitempty"`
	SSHUser        string `json:"ssh_user,omitempty"`
	SSHPasswordEnc string `json:"ssh_password_enc,omitempty"`

	// Filtre tags (virgule-séparés, vide = tout inclure)
	FilterTags string `json:"filter_tags,omitempty"`
}

func GetSource(sourceType, configJSON, secretKey string) (Source, error) {
	switch sourceType {
	case "manual":
		return &ManualSource{}, nil
	case "proxmox":
		var stored StoredProxmoxConfig
		if err := json.Unmarshal([]byte(configJSON), &stored); err != nil {
			return nil, fmt.Errorf("config proxmox invalide: %w", err)
		}

		cfg := ProxmoxConfig{
			AuthMode:   stored.AuthMode,
			URL:        stored.URL,
			User:       stored.User,
			Node:       stored.Node,
			Insecure:   stored.Insecure,
			SSHHost:    stored.SSHHost,
			SSHPort:    stored.SSHPort,
			SSHUser:    stored.SSHUser,
			FilterTags: splitTags(stored.FilterTags),
			APITokenID: stored.APITokenID,
		}

		if stored.PasswordEnc != "" {
			plain, err := crypto.Decrypt(secretKey, stored.PasswordEnc)
			if err != nil {
				return nil, fmt.Errorf("déchiffrement mot de passe API proxmox: %w", err)
			}
			cfg.Password = plain
		}

		if stored.SSHPasswordEnc != "" {
			plain, err := crypto.Decrypt(secretKey, stored.SSHPasswordEnc)
			if err != nil {
				return nil, fmt.Errorf("déchiffrement mot de passe SSH proxmox: %w", err)
			}
			cfg.SSHPassword = plain
		}

		if stored.APITokenSecretEnc != "" {
			plain, err := crypto.Decrypt(secretKey, stored.APITokenSecretEnc)
			if err != nil {
				return nil, fmt.Errorf("déchiffrement secret token API proxmox: %w", err)
			}
			cfg.APITokenSecret = plain
		}

		return &ProxmoxSource{Config: cfg}, nil
	case "ad":
		return nil, fmt.Errorf("source ActiveDirectory non encore implémentée")
	case "ocs":
		return nil, fmt.Errorf("source OCSInventory non encore implémentée")
	case "vcenter":
		return nil, fmt.Errorf("source vCenter non encore implémentée")
	default:
		return nil, fmt.Errorf("source inconnue: %s", sourceType)
	}
}

type ManualSource struct{}

func (m *ManualSource) Sync() ([]HostData, []GroupData, error) {
	return []HostData{}, []GroupData{}, nil
}

// splitTags découpe une chaîne de tags séparés par des virgules.
func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	var tags []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}
