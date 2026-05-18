package main

import (
	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/commands"
	"github.com/pedroborgesdev/tunnerse-cli/internal/cli/utils"
)

func main() {
	// Garante que o terminal seja restaurado ao final
	defer utils.EnableInput()

	// Comentado temporariamente para evitar problemas com o terminal
	utils.DisableInput()

	commands.Execute()
}
