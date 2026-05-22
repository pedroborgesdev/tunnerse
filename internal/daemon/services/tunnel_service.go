package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/config"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/jobs"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/logger"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/models"
)

type TunnelService struct{}

var tunnelServiceHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}

const tunnelHeartbeatTimeout = 10 * time.Second

var (
	tunnelHeartbeatMu            sync.Mutex
	tunnelHeartbeatTimers        = map[string]*time.Timer{}
	tunnelHeartbeatGeneration    = map[string]uint64{}
	tcpTunnelHeartbeatMu         sync.Mutex
	tcpTunnelHeartbeatTimers     = map[string]*time.Timer{}
	tcpTunnelHeartbeatGeneration = map[string]uint64{}
)

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
	s.resetTunnelHeartbeatMonitor(tunnelID)

	go func() {
		loopJob.StartTunnelLoop()
		s.stopTunnelHeartbeatMonitor(tunnelID)
		config.RemoveActiveJob(tunnelID)
	}()

	return tunnelID, result.Data.Subdomain, nil
}

func (s *TunnelService) RegisterTCPTunnel(name, port, serverURL string) (string, string, error) {
	wsURL, err := buildTCPWebSocketURL(serverURL)
	if err != nil {
		return "", "", err
	}

	job := jobs.NewTCPTunnelJob(name, port, wsURL)
	hostname, err := job.Start()
	if err != nil {
		return "", "", err
	}

	tunnelID := extractTCPHostnameName(hostname)
	if tunnelID == "" {
		job.Stop()
		return "", "", fmt.Errorf("TCP register response produced empty tunnel id")
	}

	endpoint := hostname + ":443"
	config.SetTCPTunnelEndpoint(tunnelID, endpoint)
	config.SetActiveJob(tunnelID, job)
	s.resetTCPTunnelHeartbeatMonitor(tunnelID)

	go func() {
		<-job.Done()
		s.stopTCPTunnelHeartbeatMonitor(tunnelID)
		config.RemoveActiveJob(tunnelID)
		config.RemoveTCPTunnelEndpoint(tunnelID)
	}()

	return tunnelID, endpoint, nil
}

func (s *TunnelService) KeepTCPTunnelAlive(tunnelID string) error {
	if _, exists := config.GetTCPTunnelEndpoint(tunnelID); !exists {
		return fmt.Errorf("TCP tunnel not found")
	}

	if _, exists := config.GetActiveJob(tunnelID); !exists {
		return fmt.Errorf("TCP tunnel not found")
	}

	s.resetTCPTunnelHeartbeatMonitor(tunnelID)
	return nil
}

func (s *TunnelService) StopTCPTunnel(tunnelID string) error {
	if _, exists := config.GetTCPTunnelEndpoint(tunnelID); !exists {
		return fmt.Errorf("TCP tunnel not found")
	}

	s.stopTCPTunnelHeartbeatMonitor(tunnelID)

	if job, exists := config.GetActiveJob(tunnelID); exists {
		config.RemoveActiveJob(tunnelID)
		job.Stop()
	}
	config.RemoveTCPTunnelEndpoint(tunnelID)

	return nil
}

func (s *TunnelService) KeepHTTPTunnelAlive(tunnelID string) error {
	if _, exists := config.GetHTTPTunnelURL(tunnelID); !exists {
		return fmt.Errorf("HTTP tunnel not found")
	}

	if _, exists := config.GetActiveJob(tunnelID); !exists {
		return fmt.Errorf("HTTP tunnel not found")
	}

	s.resetTunnelHeartbeatMonitor(tunnelID)
	return nil
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

	s.stopTunnelHeartbeatMonitor(tunnelID)

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

func (s *TunnelService) resetTunnelHeartbeatMonitor(tunnelID string) {
	tunnelHeartbeatMu.Lock()
	tunnelHeartbeatGeneration[tunnelID]++
	generation := tunnelHeartbeatGeneration[tunnelID]

	if timer, exists := tunnelHeartbeatTimers[tunnelID]; exists {
		timer.Stop()
	}

	tunnelHeartbeatTimers[tunnelID] = time.AfterFunc(tunnelHeartbeatTimeout, func() {
		tunnelHeartbeatMu.Lock()
		if tunnelHeartbeatGeneration[tunnelID] != generation {
			tunnelHeartbeatMu.Unlock()
			return
		}
		delete(tunnelHeartbeatTimers, tunnelID)
		delete(tunnelHeartbeatGeneration, tunnelID)
		tunnelHeartbeatMu.Unlock()

		logger.Log("FATAL", "CLI heartbeat timeout, closing tunnel", []logger.LogDetail{
			{Key: "tunnel_id", Value: tunnelID},
			{Key: "timeout", Value: tunnelHeartbeatTimeout.String()},
		})

		if err := s.StopHTTPTunnel(tunnelID); err != nil {
			logger.Log("ERROR", "Failed to stop HTTP tunnel after heartbeat timeout", []logger.LogDetail{
				{Key: "tunnel_id", Value: tunnelID},
				{Key: "Error", Value: err.Error()},
			})
		}
	})
	tunnelHeartbeatMu.Unlock()
}

func (s *TunnelService) stopTunnelHeartbeatMonitor(tunnelID string) {
	tunnelHeartbeatMu.Lock()
	defer tunnelHeartbeatMu.Unlock()

	if timer, exists := tunnelHeartbeatTimers[tunnelID]; exists {
		timer.Stop()
		delete(tunnelHeartbeatTimers, tunnelID)
	}
	delete(tunnelHeartbeatGeneration, tunnelID)
}

func (s *TunnelService) resetTCPTunnelHeartbeatMonitor(tunnelID string) {
	tcpTunnelHeartbeatMu.Lock()
	tcpTunnelHeartbeatGeneration[tunnelID]++
	generation := tcpTunnelHeartbeatGeneration[tunnelID]

	if timer, exists := tcpTunnelHeartbeatTimers[tunnelID]; exists {
		timer.Stop()
	}

	tcpTunnelHeartbeatTimers[tunnelID] = time.AfterFunc(tunnelHeartbeatTimeout, func() {
		tcpTunnelHeartbeatMu.Lock()
		if tcpTunnelHeartbeatGeneration[tunnelID] != generation {
			tcpTunnelHeartbeatMu.Unlock()
			return
		}
		delete(tcpTunnelHeartbeatTimers, tunnelID)
		delete(tcpTunnelHeartbeatGeneration, tunnelID)
		tcpTunnelHeartbeatMu.Unlock()

		logger.Log("FATAL", "CLI heartbeat timeout, closing TCP tunnel", []logger.LogDetail{
			{Key: "tunnel_id", Value: tunnelID},
			{Key: "timeout", Value: tunnelHeartbeatTimeout.String()},
		})

		if err := s.StopTCPTunnel(tunnelID); err != nil {
			logger.Log("ERROR", "Failed to stop TCP tunnel after heartbeat timeout", []logger.LogDetail{
				{Key: "tunnel_id", Value: tunnelID},
				{Key: "Error", Value: err.Error()},
			})
		}
	})
	tcpTunnelHeartbeatMu.Unlock()
}

func (s *TunnelService) stopTCPTunnelHeartbeatMonitor(tunnelID string) {
	tcpTunnelHeartbeatMu.Lock()
	defer tcpTunnelHeartbeatMu.Unlock()

	if timer, exists := tcpTunnelHeartbeatTimers[tunnelID]; exists {
		timer.Stop()
		delete(tcpTunnelHeartbeatTimers, tunnelID)
	}
	delete(tcpTunnelHeartbeatGeneration, tunnelID)
}

func extractTunnelID(fullURL, serverDomain string) string {
	url := strings.TrimPrefix(fullURL, "http://")
	url = strings.TrimPrefix(url, "https://")

	url = strings.TrimSuffix(url, "."+serverDomain)
	url = strings.TrimPrefix(url, serverDomain+"/")

	return url
}

func buildTCPWebSocketURL(serverURL string) (string, error) {
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		return "", fmt.Errorf("server_url must start with http:// or https://")
	}

	parsed, err := url.Parse(strings.TrimRight(serverURL, "/"))
	if err != nil {
		return "", err
	}

	scheme := "ws"
	if parsed.Scheme == "https" {
		scheme = "wss"
	}

	host := parsed.Host
	hostname := parsed.Hostname()
	if hostname != "" &&
		hostname != "localhost" &&
		!strings.HasPrefix(hostname, "127.") &&
		!strings.HasPrefix(hostname, "[") &&
		!strings.HasPrefix(hostname, "api.") {
		if port := parsed.Port(); port != "" {
			host = "api." + hostname + ":" + port
		} else {
			host = "api." + hostname
		}
	}

	return fmt.Sprintf("%s://%s/ws/tcp", scheme, host), nil
}

func extractTCPHostnameName(hostname string) string {
	hostname = strings.TrimPrefix(hostname, "tcp://")
	hostname = strings.TrimPrefix(hostname, "tls://")
	if h, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = h
	}
	parts := strings.Split(hostname, ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
