package config

import (
	"os"
	"path/filepath"
)

func GetUserDataDir() string {
	if dataDir := os.Getenv("TUNNERSE_DATA_DIR"); dataDir != "" {
		return dataDir
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".tunnerse"
	}
	return filepath.Join(homeDir, ".tunnerse")
}

func GetLogsDir() string {
	return filepath.Join(GetUserDataDir(), "logs")
}
