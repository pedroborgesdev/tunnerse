package logger

import (
	"fmt"
	"os"
	"time"

	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/utils"
)

type LogDetail struct {
	Key   string
	Value interface{}
}

func Log(level string, message string, details []LogDetail, showTime bool) {
	timestamp := time.Now().Format("2006/01/02-15:04:05")
	color := getLevelColor(level)
	emoji := getLevelEmoji(level)
	reset := "\033[0m"

	if showTime {
		fmt.Printf("%s%s [%s] %s%s",
			color, emoji, timestamp, message, reset)
	} else {
		fmt.Printf("%s%s %s%s",
			color, emoji, message, reset)
	}

	for _, detail := range details {
		fmt.Printf("\n%s%s%s: %v%s", color, detail.Key, reset, detail.Value, reset)
	}
	fmt.Println()

	if level == "FATAL" {
		fmt.Println()
		utils.EnableInput()
		os.Exit(1)
	}

	fmt.Println()
}

func getLevelColor(level string) string {
	switch level {
	case "DEBUG":
		return "\033[36m"
	case "INFO":
		return "\033[36m"
	case "SUCCESS":
		return "\033[32m"
	case "WARN":
		return "\033[33m"
	case "HEALTHCHECK":
		return "\033[38;2;255;105;180m"
	case "ERROR":
		return "\033[31m"
	case "FATAL":
		return "\033[31m"
	default:
		return "\033[35m"
	}
}

func getLevelEmoji(level string) string {
	switch level {
	case "DEBUG":
		return "[•]"
	case "INFO":
		return "[i]"
	case "SUCCESS":
		return "[✓]"
	case "WARN":
		return "[!]"
	case "HEALTHCHECK":
		return "[♥]"
	case "ERROR":
		return "[✗]"
	case "FATAL":
		return "[†]"
	default:
		return "[*]"
	}
}
