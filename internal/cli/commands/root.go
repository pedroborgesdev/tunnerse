package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/dto"
	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/utils"

	"github.com/spf13/cobra"
)

var (
	ForApp bool
)

var rootCmd = &cobra.Command{
	Use:   "tunnerse",
	Short: "Expose your on-premises application to the entire internet via reverse tunnels",
	Run: func(cmd *cobra.Command, args []string) {
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	utils.Clear()

	fmt.Print(dto.Welcome)
	fmt.Print(dto.Banner)
	rootCmd.AddCommand(httpTunnel)
	if err := rootCmd.Execute(); err != nil {
		utils.Clear()

		fmt.Print(dto.Welcome)
		fmt.Print(dto.Banner)
		fmt.Printf("%s\n", friendlyCommandError(err))
		fmt.Print(dto.Commands)
		os.Exit(1)
	}

	fmt.Print(dto.Commands)

}

func friendlyCommandError(err error) string {
	message := err.Error()
	if strings.HasPrefix(message, "unknown command") {
		return "Command does not exist: " + message
	}
	if strings.Contains(message, "incorrect usage") {
		return "Command used incorrectly: " + message
	}
	return message
}
