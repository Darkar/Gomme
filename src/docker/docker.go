package docker

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client communique avec le Docker daemon via socket-proxy (HTTP).
type Client struct {
	base       string       // ex: "http://socket-proxy:2375"
	http       *http.Client // timeout standard
	httpStream *http.Client // sans timeout (streaming de logs)
}

func New(dockerHost string) *Client {
	// Convertit "tcp://host:port" → "http://host:port"
	// Laisse "http://..." tel quel
	base := dockerHost
	if strings.HasPrefix(base, "tcp://") {
		base = "http://" + strings.TrimPrefix(base, "tcp://")
	} else if strings.HasPrefix(base, "unix://") || strings.HasPrefix(base, "unix:///") {
		base = "http://localhost"
	}
	// Supprimer le slash final
	base = strings.TrimRight(base, "/")

	return &Client{
		base: base,
		http: &http.Client{Timeout: 30 * time.Second},
		httpStream: &http.Client{
			Timeout: 0, // pas de timeout pour le streaming
		},
	}
}

// Image représente une image Docker (résultat de /images/json).
type Image struct {
	ID          string   `json:"Id"`
	RepoTags    []string `json:"RepoTags"`
	RepoDigests []string `json:"RepoDigests"`
	Size        int64    `json:"Size"`
}

// ListImages retourne toutes les images disponibles sur le daemon.
func (c *Client) ListImages() ([]Image, error) {
	resp, err := c.http.Get(c.base + "/images/json")
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list images: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var images []Image
	if err := json.NewDecoder(resp.Body).Decode(&images); err != nil {
		return nil, fmt.Errorf("list images decode: %w", err)
	}
	return images, nil
}

// ContainerConfig décrit le conteneur à créer.
type ContainerConfig struct {
	Image       string
	Cmd         []string
	Env         []string
	VolumesFrom []string // ex: ["gomme-app"] pour hériter de /app/repo
	WorkingDir  string
	AutoRemove  bool
}

type createRequest struct {
	Image      string           `json:"Image"`
	Cmd        []string         `json:"Cmd"`
	Env        []string         `json:"Env"`
	WorkingDir string           `json:"WorkingDir,omitempty"`
	HostConfig createHostConfig `json:"HostConfig"`
}

type createHostConfig struct {
	VolumesFrom []string `json:"VolumesFrom,omitempty"`
	AutoRemove  bool     `json:"AutoRemove"`
}

// CreateContainer crée un conteneur et retourne son ID.
func (c *Client) CreateContainer(cfg ContainerConfig) (string, error) {
	payload := createRequest{
		Image:      cfg.Image,
		Cmd:        cfg.Cmd,
		Env:        cfg.Env,
		WorkingDir: cfg.WorkingDir,
		HostConfig: createHostConfig{
			VolumesFrom: cfg.VolumesFrom,
			AutoRemove:  cfg.AutoRemove,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Post(c.base+"/containers/create", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create container: HTTP %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.ID, nil
}

// StartContainer démarre un conteneur existant.
func (c *Client) StartContainer(id string) error {
	resp, err := c.http.Post(c.base+"/containers/"+id+"/start", "application/json", nil)
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("start container: HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// StreamLogs lit les logs d'un conteneur en streaming et appelle cb pour chaque ligne.
// Le format Docker multiplexé : header 8 octets (1=stdout,2=stderr + 3 reserved + 4 size), puis données.
func (c *Client) StreamLogs(id string, cb func(string)) error {
	url := c.base + "/containers/" + id + "/logs?follow=true&stdout=true&stderr=true"
	resp, err := c.httpStream.Get(url)
	if err != nil {
		return fmt.Errorf("stream logs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stream logs: HTTP %d: %s", resp.StatusCode, string(b))
	}

	header := make([]byte, 8)
	for {
		_, err := io.ReadFull(resp.Body, header)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return err
		}
		size := binary.BigEndian.Uint32(header[4:8])
		if size == 0 {
			continue
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(resp.Body, data); err != nil {
			break
		}
		cb(string(data))
	}
	return nil
}

// WaitContainer attend la fin d'un conteneur et retourne son exit code.
func (c *Client) WaitContainer(id string) (int64, error) {
	resp, err := c.httpStream.Post(c.base+"/containers/"+id+"/wait", "application/json", nil)
	if err != nil {
		return -1, fmt.Errorf("wait container: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		StatusCode int64 `json:"StatusCode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return -1, err
	}
	return result.StatusCode, nil
}

// LogsReader retourne un ReadCloser sur les logs du conteneur.
// Si follow=true, la connexion reste ouverte jusqu'à la fin du conteneur.
func (c *Client) LogsReader(id string, follow bool) (io.ReadCloser, error) {
	followStr := "false"
	if follow {
		followStr = "true"
	}
	u := c.base + "/containers/" + id + "/logs?follow=" + followStr + "&stdout=true&stderr=true"
	httpClient := c.http
	if follow {
		httpClient = c.httpStream
	}
	resp, err := httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("logs reader: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("logs reader: HTTP %d: %s", resp.StatusCode, string(b))
	}
	return resp.Body, nil
}

// RemoveContainer supprime un conteneur (force=true).
func (c *Client) RemoveContainer(id string) error {
	req, err := http.NewRequest(http.MethodDelete, c.base+"/containers/"+id+"?force=true", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("remove container: %w", err)
	}
	defer resp.Body.Close()
	return nil
}
