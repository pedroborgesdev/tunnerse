package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/logger"
	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/text"
	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/utils"
	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/validators"
	"github.com/spf13/cobra"
)

var tcpTunnel = &cobra.Command{
	Use:   "tcp <tunnel_name> <local_port>",
	Short: "Start a TCP tunnel on current terminal",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		startTCPTunnel(args)
	},
}

type TCPResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Tunnel   string `json:"tunnel"`
		Endpoint string `json:"endpoint"`
	} `json:"data"`
	Status int `json:"status"`
}

func startTCPTunnel(args []string) {
	fmt.Printf(text.Start)

	validateTCPArgs(args)

	tunnelName := args[0]
	port := args[1]
	serverURL := "https://tunnerse.com"

	payload := map[string]string{
		"name":       tunnelName,
		"port":       port,
		"server_url": serverURL,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		logger.Log("FATAL", "Failed to create request payload", []logger.LogDetail{
			{Key: "Error", Value: err.Error()},
		}, false)
	}

	resp, err := localServerHTTPClient.Post("http://localhost:9988/tcp", "application/json", bytes.NewBuffer(data))
	if err != nil {
		if isConnRefused(err) {
			logger.Log("FATAL", "Tunnerse local server is not online", []logger.LogDetail{
				{Key: "Hint", Value: "Make sure tunnerse-daemon is running and accessible on http://localhost:9988"},
			}, false)
		} else {
			logger.Log("FATAL", "Failed to connect to local API", []logger.LogDetail{
				{Key: "Error", Value: err.Error()},
			}, false)
		}
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		logger.Log("FATAL", "Failed to read daemon response", []logger.LogDetail{
			{Key: "Error", Value: err.Error()},
		}, false)
	}

	var result TCPResponse
	if err := json.Unmarshal(body, &result); err != nil {
		logger.Log("FATAL", "Failed to decode daemon response", []logger.LogDetail{
			{Key: "Error", Value: err.Error()},
			{Key: "Status", Value: resp.StatusCode},
			{Key: "Body", Value: responsePreview(body)},
		}, false)
	}

	if resp.StatusCode != http.StatusOK || result.Status != http.StatusOK {
		logger.Log("FATAL", "Daemon returned error", []logger.LogDetail{
			{Key: "Message", Value: result.Message},
			{Key: "Code", Value: result.Code},
			{Key: "HTTP Status", Value: resp.StatusCode},
			{Key: "Body", Value: responsePreview(body)},
		}, false)
	}

	tunnelID := result.Data.Tunnel
	endpoint := result.Data.Endpoint

	logger.Log("SUCCESS", "TCP tunnel created successfully!", []logger.LogDetail{
		{Key: "Tunnel Name", Value: tunnelID},
		{Key: "Public Endpoint", Value: endpoint},
	}, false)
	logger.Log("WARN", "Press Ctrl+C to stop", []logger.LogDetail{}, false)

	time.Sleep(500 * time.Millisecond)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	stopBackground := make(chan struct{})
	go keepTCPTunnelAlive(tunnelID, stopBackground)

	<-sigChan
	close(stopBackground)

	fmt.Println()
	logger.Log("INFO", "Stopping TCP tunnel...", []logger.LogDetail{}, false)
	stopTCPTunnel(tunnelID)
	logger.Log("SUCCESS", "TCP tunnel stopped", []logger.LogDetail{}, false)

	restoreTerminalAndExitTCP(1)
}

func stopTCPTunnel(tunnelID string) {
	payload := map[string]string{
		"tunnel_id": tunnelID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		logger.Log("ERROR", "Failed to create TCP stop request", []logger.LogDetail{
			{Key: "Error", Value: err.Error()},
		}, false)
		return
	}

	resp, err := localServerHTTPClient.Post("http://localhost:9988/tcp/stop", "application/json", bytes.NewBuffer(data))
	if err != nil {
		if isConnRefused(err) {
			logger.Log("FATAL", "Tunnerse local server is not online", []logger.LogDetail{
				{Key: "Hint", Value: "Make sure tunnerse-daemon is running and accessible on http://localhost:9988"},
			}, false)
		} else {
			logger.Log("ERROR", "Failed to stop TCP tunnel", []logger.LogDetail{
				{Key: "Error", Value: err.Error()},
			}, false)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		logger.Log("ERROR", "Failed to stop TCP tunnel", []logger.LogDetail{
			{Key: "HTTP Status", Value: resp.StatusCode},
			{Key: "Body", Value: strings.TrimSpace(string(body))},
		}, false)
	}
}

func keepTCPTunnelAlive(tunnelID string, done <-chan struct{}) {
	for {
		if err := sendTCPTunnelHeartbeat(tunnelID); err != nil {
			if isConnRefused(err) {
				logger.Log("ERROR", "Tunnerse local server is not online", []logger.LogDetail{
					{Key: "Hint", Value: "Make sure tunnerse-daemon is running and accessible on http://localhost:9988"},
				}, false)
			}
			return
		}

		select {
		case <-done:
			return
		case <-time.After(tunnelHeartbeatInterval):
		}
	}
}

func sendTCPTunnelHeartbeat(tunnelID string) error {
	endpoint := fmt.Sprintf("http://localhost:9988/tcp/health/%s", url.PathEscape(tunnelID))

	resp, err := localServerHTTPClient.Get(endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("TCP heartbeat rejected: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func validateTCPArgs(args []string) {
	validator := validators.NewArgsValidator()

	if err := validator.ValidateExposeArgs(args[0], args[1]); err != nil {
		logger.Log("FATAL", "Invalid arguments", []logger.LogDetail{
			{Key: "Error", Value: err.Error()},
		}, false)
	}
}

func restoreTerminalAndExitTCP(code int) {
	utils.EnableInput()
	os.Exit(code)
}
