package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/dto"
	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/logger"
	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/utils"
	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/validators"

	"github.com/spf13/cobra"
)

var httpTunnel = &cobra.Command{
	Use:   "http <tunnel_name> <local_port>",
	Short: "Start an HTTP tunnel on current terminal (no database)",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		startHTTPTunnel(args)
	},
}

type HTTPResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Tunnel    string `json:"tunnel"`
		Subdomain bool   `json:"subdomain"`
	} `json:"data"`
	Status int `json:"status"`
}

type LogResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TunnelID string `json:"tunnel_id"`
		Logs     string `json:"logs"`
		Offset   int64  `json:"offset"`
	} `json:"data"`
	Status int `json:"status"`
}

var localServerHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}

func startHTTPTunnel(args []string) {
	fmt.Printf(dto.Start)

	validateHTTPArgs(args)

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

	resp, err := localServerHTTPClient.Post("http://localhost:9988/http", "application/json", bytes.NewBuffer(data))
	if err != nil {
		if isConnRefused(err) {
			logger.Log("FATAL", "Tunnerse local server is not online", []logger.LogDetail{
				{Key: "Hint", Value: "Make sure tunnerse-server is running and accessible on http://localhost:9988"},
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
		logger.Log("FATAL", "Failed to read server response", []logger.LogDetail{
			{Key: "Error", Value: err.Error()},
		}, false)
	}

	var result HTTPResponse
	if err := json.Unmarshal(body, &result); err != nil {
		logger.Log("FATAL", "Failed to decode server response", []logger.LogDetail{
			{Key: "Error", Value: err.Error()},
			{Key: "Status", Value: resp.StatusCode},
			{Key: "Body", Value: responsePreview(body)},
		}, false)
	}

	if resp.StatusCode != http.StatusOK || result.Status != http.StatusOK {
		logger.Log("FATAL", "Server returned error", []logger.LogDetail{
			{Key: "Message", Value: result.Message},
			{Key: "Code", Value: result.Code},
			{Key: "HTTP Status", Value: resp.StatusCode},
			{Key: "Body", Value: responsePreview(body)},
		}, false)
	}

	tunnelID := result.Data.Tunnel
	isSubdomain := result.Data.Subdomain

	serverDomain := strings.TrimPrefix(serverURL, "http://")
	serverDomain = strings.TrimPrefix(serverDomain, "https://")

	protocol := "http://"
	if strings.HasPrefix(serverURL, "https://") {
		protocol = "https://"
	}

	var tunnelURL string
	if isSubdomain {
		tunnelURL = fmt.Sprintf("%s%s.%s", protocol, tunnelID, serverDomain)
	} else {
		tunnelURL = fmt.Sprintf("%s%s/%s", protocol, serverDomain, tunnelID)
	}

	logger.Log("SUCCESS", "HTTP tunnel created successfully!", []logger.LogDetail{
		{Key: "Tunnel URL", Value: tunnelURL},
	}, false)
	logger.Log("WARN", "Press Ctrl+C to stop", []logger.LogDetail{}, false)

	time.Sleep(500 * time.Millisecond)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	stopLogs := make(chan struct{})
	go tailTunnelLogs(tunnelID, stopLogs)

	<-sigChan
	close(stopLogs)

	fmt.Println()
	logger.Log("INFO", "Stopping tunnel...", []logger.LogDetail{}, false)
	stopTunnel(tunnelID)
	logger.Log("SUCCESS", "HTTP tunnel stopped", []logger.LogDetail{}, false)

	restoreTerminalAndExitHTTP(1)
}

func stopTunnel(tunnelID string) {
	payload := map[string]string{
		"tunnel_id": tunnelID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		logger.Log("ERROR", "Failed to create stop request", []logger.LogDetail{
			{Key: "Error", Value: err.Error()},
		}, false)
		return
	}

	resp, err := localServerHTTPClient.Post("http://localhost:9988/http/stop", "application/json", bytes.NewBuffer(data))
	if err != nil {
		if isConnRefused(err) {
			logger.Log("FATAL", "Tunnerse local server is not online", []logger.LogDetail{
				{Key: "Hint", Value: "Make sure tunnerse-server is running and accessible on http://localhost:9988"},
			}, false)
		} else {
			logger.Log("ERROR", "Failed to stop tunnel", []logger.LogDetail{
				{Key: "Error", Value: err.Error()},
			}, false)
		}
		return
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log("ERROR", "Failed to read response", []logger.LogDetail{
			{Key: "Error", Value: err.Error()},
		}, false)
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		logger.Log("ERROR", "Failed to parse response", []logger.LogDetail{
			{Key: "Error", Value: err.Error()},
		}, false)
		return
	}

	if resp.StatusCode != 200 {
		var errMsg interface{}
		if data, ok := result["data"].(map[string]interface{}); ok {
			errMsg = data["error"]
		}
		logger.Log("ERROR", "Failed to stop tunnel", []logger.LogDetail{
			{Key: "Error", Value: fmt.Sprintf("%v", errMsg)},
		}, false)
		return
	}
}

func tailTunnelLogs(tunnelID string, done <-chan struct{}) {
	offset := int64(-1)

	for {
		select {
		case <-done:
			return
		default:
		}

		result, statusCode, err := fetchTunnelLogs(tunnelID, offset)
		if err != nil {
			if isConnRefused(err) {
				logger.Log("ERROR", "Tunnerse local server is not online", []logger.LogDetail{
					{Key: "Hint", Value: "Make sure tunnerse-server is running and accessible on http://localhost:9988"},
				}, false)
				return
			}
			waitForNextLogPoll(done)
			continue
		}

		if statusCode == http.StatusOK && result.Status == http.StatusOK {
			if result.Data.Logs != "" {
				fmt.Print(filterTunnelLogsForCLI(result.Data.Logs))
			}
			offset = result.Data.Offset
		}

		waitForNextLogPoll(done)
	}
}

func fetchTunnelLogs(tunnelID string, offset int64) (LogResponse, int, error) {
	var result LogResponse
	endpoint := fmt.Sprintf(
		"http://localhost:9988/http/logs/%s?offset=%d",
		url.PathEscape(tunnelID),
		offset,
	)

	resp, err := localServerHTTPClient.Get(endpoint)
	if err != nil {
		return result, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, resp.StatusCode, err
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return result, resp.StatusCode, err
	}

	return result, resp.StatusCode, nil
}

func filterTunnelLogsForCLI(logs string) string {
	var output strings.Builder
	for _, entry := range splitTunnelLogEntries(logs) {
		if isVisibleTunnelLogEntry(entry) {
			output.WriteString(entry)
		}
	}
	return output.String()
}

func splitTunnelLogEntries(logs string) []string {
	if logs == "" {
		return nil
	}

	var entries []string
	var current strings.Builder
	for _, line := range strings.SplitAfter(logs, "\n") {
		if strings.HasPrefix(line, "[tunnerse-server]") && current.Len() > 0 {
			entries = append(entries, current.String())
			current.Reset()
		}
		current.WriteString(line)
	}

	if current.Len() > 0 {
		entries = append(entries, current.String())
	}

	return entries
}

func isVisibleTunnelLogEntry(entry string) bool {
	for _, line := range strings.Split(entry, "\n") {
		cleanLine := stripANSISequences(line)
		if strings.Contains(cleanLine, "INFO - ") ||
			strings.Contains(cleanLine, "WARN - ") ||
			strings.Contains(cleanLine, "FATAL - ") {
			return true
		}
	}
	return false
}

func stripANSISequences(value string) string {
	var output strings.Builder
	inEscape := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if inEscape {
			if ch >= '@' && ch <= '~' {
				inEscape = false
			}
			continue
		}
		if ch == '\x1b' {
			inEscape = true
			continue
		}
		output.WriteByte(ch)
	}
	return output.String()
}

func waitForNextLogPoll(done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}
}

func validateHTTPArgs(args []string) {
	validator := validators.NewArgsValidator()

	if err := validator.ValidateExposeArgs(args[0], args[1]); err != nil {
		logger.Log("FATAL", "Invalid arguments", []logger.LogDetail{
			{Key: "Error", Value: err.Error()},
		}, false)
	}
}

func restoreTerminalAndExitHTTP(code int) {
	utils.EnableInput()
	os.Exit(code)
}

func responsePreview(body []byte) string {
	const maxPreviewSize = 500
	text := strings.TrimSpace(string(body))
	if len(text) > maxPreviewSize {
		return text[:maxPreviewSize] + "..."
	}
	if text == "" {
		return "<empty>"
	}
	return text
}

func isConnRefused(err error) bool {
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Err != nil && strings.Contains(netErr.Err.Error(), "connection refused") {
			return true
		}
	}
	if strings.Contains(err.Error(), "connection refused") {
		return true
	}
	return false
}
