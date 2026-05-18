package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/config"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/jobs"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/models"
)

type TunnelService struct{}

var tunnelServiceHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}

func NewTunnelService() *TunnelService {
	return &TunnelService{}
}

func (s *TunnelService) RegisterTunnel(name, port, server_url string) (string, bool, error) {
	if !strings.HasPrefix(server_url, "http://") && !strings.HasPrefix(server_url, "https://") {
		return "", false, fmt.Errorf("server_url must start with http:// or https://")
	}
	server_url = strings.TrimRight(server_url, "/")

	payload := map[string]string{"name": name}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", false, fmt.Errorf("encode JSON: %w", err)
	}

	registerURL := fmt.Sprintf("%s/register", server_url)
	resp, err := tunnelServiceHTTPClient.Post(registerURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", false, fmt.Errorf("post register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", false, fmt.Errorf("register rejected: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result models.RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", false, fmt.Errorf("decode register response. probably tunnerse server is offline: %w", err)
	}

	tunnelFullURL := result.Data.Tunnel
	if tunnelFullURL == "" {
		return "", false, fmt.Errorf("register response missing tunnel")
	}

	serverDomain := strings.TrimPrefix(server_url, "http://")
	serverDomain = strings.TrimPrefix(serverDomain, "https://")

	tunnelID := extractTunnelID(tunnelFullURL, serverDomain)
	if tunnelID == "" {
		return "", false, fmt.Errorf("register response produced empty tunnel id")
	}

	protocol := "http://"
	if strings.HasPrefix(server_url, "https://") {
		protocol = "https://"
	}

	var finalTunnelURL string
	if strings.HasPrefix(tunnelFullURL, "http://") || strings.HasPrefix(tunnelFullURL, "https://") {
		finalTunnelURL = tunnelFullURL
	} else {
		if result.Data.Subdomain {
			finalTunnelURL = fmt.Sprintf("%s%s.%s", protocol, tunnelFullURL, serverDomain)
		} else {
			finalTunnelURL = fmt.Sprintf("%s%s/%s", protocol, serverDomain, tunnelFullURL)
		}
	}

	config.SetHTTPTunnelURL(tunnelID, finalTunnelURL)

	loopJob := jobs.NewLoopJob(tunnelID, port, result.Data.Subdomain, finalTunnelURL)
	if loopJob == nil {
		return "", false, fmt.Errorf("failed to create tunnel job")
	}

	config.SetActiveJob(tunnelID, loopJob)

	go func() {
		loopJob.StartTunnelLoop()
		config.RemoveActiveJob(tunnelID)
	}()

	return tunnelID, result.Data.Subdomain, nil
}

func (s *TunnelService) StopHTTPTunnel(tunnelID string) error {
	var tunnelURL string

	if url, exists := config.GetHTTPTunnelURL(tunnelID); exists {
		tunnelURL = url
	} else {
		return fmt.Errorf("HTTP tunnel not found")
	}

	if tunnelURL == "" {
		return fmt.Errorf("tunnel URL is empty")
	}

	if job, exists := config.GetActiveJob(tunnelID); exists {
		config.RemoveActiveJob(tunnelID)
		job.Stop()
	}
	config.RemoveHTTPTunnelURL(tunnelID)

	go func() {
		closeURL := tunnelURL + "/close"
		payload := map[string]string{"name": tunnelID}
		data, err := json.Marshal(payload)
		if err != nil {
			fmt.Printf("failed to marshal close payload: %v\n", err)
			return
		}

		resp, err := tunnelServiceHTTPClient.Post(closeURL, "application/json", bytes.NewBuffer(data))
		if err != nil {
			fmt.Printf("failed to send close request: %v\n", err)
			return
		}
		defer resp.Body.Close()
	}()

	return nil
}

func extractTunnelID(fullURL, serverDomain string) string {
	url := strings.TrimPrefix(fullURL, "http://")
	url = strings.TrimPrefix(url, "https://")

	url = strings.TrimSuffix(url, "."+serverDomain)
	url = strings.TrimPrefix(url, serverDomain+"/")

	return url
}
