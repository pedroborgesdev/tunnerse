package text

import (
	"fmt"

	"github.com/pedroborgesdev/tunnerse-cli/internal/version"
)

var PURPLE = "\033[38;5;141m"
var GREEN = "\033[38;5;118m"
var RESET = "\033[0m"

var Banner = fmt.Sprintf(`
%s████████╗██╗   ██╗███╗   ██╗███╗   ██╗███████╗██████╗ ███████╗███████╗
╚══██╔══╝██║   ██║████╗  ██║████╗  ██║██╔════╝██╔══██╗██╔════╝██╔════╝
   ██║   ██║   ██║██╔██╗ ██║██╔██╗ ██║█████╗  ██████╔╝███████╗█████╗  
   ██║   ██║   ██║██║╚██╗██║██║╚██╗██║██╔══╝  ██╔══██╗╚════██║██╔══╝  
   ██║   ╚██████╔╝██║ ╚████║██║ ╚████║███████╗██║  ██║███████║███████╗
   ╚═╝    ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═══╝╚══════╝╚═╝  ╚═╝╚══════╝╚══════╝
                                                                
%s                                        +-----   
+++++++++++++++++++++++++++++++++++++++######+   
#############################################+   
++++++++++++++++++++++++++++++++++++++###+++++   %sby: pedroborgesdev%v
+++++++++++++++++++++++++++++++++++++++++++++-   %sversion: %s%v
---------------------------------------++----.   
+++++++++++++++++++++++++++++++++++++++++-----   
                                        +-----    
                                                                
%v`, GREEN, GREEN, RESET, GREEN, RESET, version.String(), GREEN, RESET)

var Info = fmt.Sprintf(`Version: %s
Author: pedroborgesdev (on GitHub)
Project: https://github.com/pedroborgesdev/tunnerse-cli.git

`, version.String())

const (
	Welcome string = `Tunnerse - Reverse tunnel manager for development and testing.
`
	Commands string = `Usage: tunnerse <command> [arguments]

Commands:
  http <name> <port>    Create a temporary tunnel (runs in foreground)

Options:
  -h, --help            Show this help message

Examples:
  tunnerse http test-app 3000

`
	Help string = `Tunnerse creates a tunnel that connects the target server using Tunnerse
Server, with your machine pointing to a local port. Tunnerse Server
acts only as an intermediary between the requester and your machine,
while Tunnerse CLI translates the request coming from the server to your
local application. The same process occurs when returning the response
from your application.

Usage: tunnerse <command> [arguments]

Commands:
  http <name> <port>    Create a temporary tunnel

Options:
  -h, --help            Show this help message

Examples:
  tunnerse http test-app 3000   # Create temporary tunnel

Thanks for using Tunnerse ;)

`

	Start string = `From now on, a tunnel will be created connecting
your local application to the entire internet. thank you
for choosing Tunnerse for this!

`
	Invalid string = `Invalid command usage, use 'help' to see valid usages
	
`

	BetaWarn string = "\033[33mBeta warn:\033[0m \n tunnel ID is passed in the URL path, which may\n" +
		" cause issues with internal navigation links. we attempt to rewrite paths\n" +
		" by placing the ID before the path. this is experimental and may fail.\n\n"

	InvalidID string = `Invalid tunnel id. correct usage should only contain lowercase letters and the special character '-'.
	
`

	InvalidPort string = `Invalid port. correct usage would be only numbers from 0 to 65535.
	
`
)
