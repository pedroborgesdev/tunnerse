package debug

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Debug struct {
	GinDebug bool
	Debug    bool
}

var DebugConfig Debug

func LoadDebugConfig() error {
	err := godotenv.Load()
	if err != nil {
		return err
	}

	DebugConfig = Debug{
		GinDebug: getEnvBool("GINDEBUG", true),
		Debug:    getEnvBool("DEBUG", true),
	}

	return nil
}

func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)

	if value == "" {
		return defaultValue
	}

	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}

	return boolValue
}
