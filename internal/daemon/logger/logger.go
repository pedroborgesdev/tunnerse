package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/debug"
)

type LogDetail struct {
	Key   string
	Value interface{}
}

var (
	logFiles      = make(map[string]*os.File)
	logsDir       = "logs"
	logMutex      sync.Mutex
	tunnelIDRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

const maxLogReadSize int64 = 64 * 1024

func SetTunnelLogFile(tunnelID, logsDir string) error {
	logMutex.Lock()
	defer logMutex.Unlock()

	if logsDir != "" {
		setLogsDir(logsDir)
	}

	if _, exists := logFiles[tunnelID]; exists {
		return nil
	}

	return openTunnelLogFileLocked(tunnelID)
}

func setLogsDir(dir string) {
	if dir == "" {
		return
	}
	logsDir = dir
}

func openTunnelLogFileLocked(tunnelID string) error {
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	logPath, err := tunnelLogPath(tunnelID)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	logFiles[tunnelID] = file

	return nil
}

func CloseTunnelLogFile(tunnelID string) {
	logMutex.Lock()
	defer logMutex.Unlock()

	if file, exists := logFiles[tunnelID]; exists {
		file.Sync()
		file.Close()
		delete(logFiles, tunnelID)
	}
}

func Log(level string, message string, details []LogDetail) {
	if !debug.DebugConfig.Debug && level == "DEBUG" {
		return
	}

	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
		line = 0
	}

	timestamp := time.Now().Format("2006/01/02 15:04:05")
	color := getLevelColor(level)
	reset := "\033[0m"

	var consoleMsg string
	if level == "DEBUG" {

		consoleMsg = fmt.Sprintf("%s %s%s:%d%s \n↳ %s%s%s - %s\n",
			timestamp, color, file, line, reset,
			color, level, reset, message)
	} else {

		consoleMsg = fmt.Sprintf("%s \n↳ %s%s%s - %s\n",
			timestamp, color, level, reset, message)
	}

	detailsMsg := ""
	for _, detail := range details {
		detailLine := fmt.Sprintf("  ↳ %s%s%s: %v\n", color, detail.Key, reset, detail.Value)
		consoleMsg += detailLine

		detailsMsg += detailLine
	}

	fmt.Print(consoleMsg)

	isTunnelLoop := strings.Contains(file, "tunnel_loop.go") || strings.Contains(file, "healthcheck.go")
	if isTunnelLoop {

		tunnelID := ""
		for _, detail := range details {
			if detail.Key == "tunnel_id" || detail.Key == "ID" {
				tunnelID = fmt.Sprintf("%v", detail.Value)
				break
			}
		}

		if tunnelID != "" {
			writeToLogFile(tunnelID, timestamp, level, file, line, message, detailsMsg)
		}
	}
}

func writeToLogFile(tunnelID, timestamp, level, file string, line int, message, details string) {
	logMutex.Lock()
	defer logMutex.Unlock()

	logFile, exists := logFiles[tunnelID]
	if !exists {
		if err := openTunnelLogFileLocked(tunnelID); err != nil {
			return
		}
		logFile = logFiles[tunnelID]
	}

	if logFile != nil {
		color := getLevelColor(level)
		reset := "\033[0m"

		var fileMsg string
		if level == "DEBUG" {

			fileMsg = fmt.Sprintf("[tunnerse-server] - %s %s%s:%d%s \n↳ %s%s%s - %s\n%s",
				timestamp, color, file, line, reset, color, level, reset, message, details)
		} else {

			fileMsg = fmt.Sprintf("[tunnerse-server] %s \n↳ %s%s%s - %s\n%s",
				timestamp, color, level, reset, message, details)
		}
		logFile.WriteString(fileMsg)
	}
}

func ReadTunnelLog(tunnelID string, offset int64) (string, int64, error) {
	logMutex.Lock()
	defer logMutex.Unlock()

	logPath, err := tunnelLogPath(tunnelID)
	if err != nil {
		return "", offset, err
	}

	file, err := os.Open(logPath)
	if err != nil {
		return "", offset, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", offset, err
	}

	size := info.Size()
	if offset < 0 || offset > size {
		return "", size, nil
	}
	if offset == size {
		return "", offset, nil
	}

	readSize := size - offset
	if readSize > maxLogReadSize {
		readSize = maxLogReadSize
	}

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return "", offset, err
	}

	data, err := io.ReadAll(io.LimitReader(file, readSize))
	if err != nil {
		return "", offset, err
	}

	nextOffset := offset + int64(len(data))
	return string(data), nextOffset, nil
}

func tunnelLogPath(tunnelID string) (string, error) {
	if !tunnelIDRegex.MatchString(tunnelID) {
		return "", fmt.Errorf("invalid tunnel id")
	}

	logPath := filepath.Join(logsDir, fmt.Sprintf("%s.log", tunnelID))
	cleanLogsDir, err := filepath.Abs(logsDir)
	if err != nil {
		return "", fmt.Errorf("resolve logs directory: %w", err)
	}
	cleanLogPath, err := filepath.Abs(logPath)
	if err != nil {
		return "", fmt.Errorf("resolve log path: %w", err)
	}
	if cleanLogPath != filepath.Join(cleanLogsDir, filepath.Base(cleanLogPath)) {
		return "", fmt.Errorf("invalid tunnel log path")
	}

	return logPath, nil
}

func getLevelColor(level string) string {
	switch level {
	case "DEBUG":
		return "\033[36m"
	case "INFO":
		return "\033[32m"
	case "WARN":
		return "\033[33m"
	case "ERROR":
		return "\033[31m"
	default:
		return "\033[35m"
	}
}
