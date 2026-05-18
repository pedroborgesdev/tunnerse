package config

import (
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/logger"
)

var (
	LogsDir string
)

func LoadAppConfig() error {
	if err := EnsureDataDirExists(); err != nil {
		logger.Log("ERROR", "failed to create data directory", []logger.LogDetail{
			{Key: "error", Value: err.Error()},
		})
		return err
	}

	LogsDir = GetLogsDir()

	return nil
}
