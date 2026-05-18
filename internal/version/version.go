package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var embeddedVersion string

var Version string

func String() string {
	if version := strings.TrimSpace(Version); version != "" {
		return version
	}
	if version := strings.TrimSpace(embeddedVersion); version != "" {
		return version
	}
	return "dev"
}
