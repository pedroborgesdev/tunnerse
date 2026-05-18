package jobs

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/utils"
)

func CloseKeyboardJob() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChan
		println()
		utils.EnableInput()
		os.Exit(0)
	}()
}
