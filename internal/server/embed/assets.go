package apiembed

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
)

import _ "embed"

//go:embed running.html
var runningHTML string

//go:embed notfound.html
var notFoundHTML string

//go:embed timeout.html
var timeoutHTML string

//go:embed localerror.html
var localErrorHTML string

//go:embed icon.png
var iconPNG []byte

//go:embed icon.webp
var iconWEBP []byte

//go:embed favicon.ico
var faviconICO []byte

var (
	assetDataOnce sync.Once
	iconPNGData   string
	faviconData   string
)

const githubIconURL = "https://raw.githubusercontent.com/pedroborgesdev/tunnerse-api/main/static/icon.webp"

func HTML(name string) ([]byte, error) {
	var html string
	switch name {
	case "running":
		html = runningHTML
	case "notfound":
		html = notFoundHTML
	case "timeout":
		html = timeoutHTML
	case "localerror":
		html = localErrorHTML
	default:
		return nil, fmt.Errorf("unknown embedded html asset: %s", name)
	}

	html = strings.ReplaceAll(html, githubIconURL, IconPNGDataURI())
	html = strings.ReplaceAll(html, "icon.webp", IconPNGDataURI())
	html = strings.ReplaceAll(html, "icon.png", IconPNGDataURI())
	html = strings.ReplaceAll(html, `type="image/webp"`, `type="image/x-icon"`)
	html = strings.ReplaceAll(html, "favicon.ico", FaviconDataURI())
	return []byte(html), nil
}

func FaviconICO() []byte {
	return append([]byte(nil), faviconICO...)
}

func IconPNG() []byte {
	return append([]byte(nil), iconPNG...)
}

func IconWEBP() []byte {
	return append([]byte(nil), iconWEBP...)
}

func IconPNGDataURI() string {
	buildAssetDataURIs()
	return iconPNGData
}

func FaviconDataURI() string {
	buildAssetDataURIs()
	return faviconData
}

func buildAssetDataURIs() {
	assetDataOnce.Do(func() {
		iconPNGData = "data:image/png;base64," + base64.StdEncoding.EncodeToString(iconPNG)
		faviconData = "data:image/x-icon;base64," + base64.StdEncoding.EncodeToString(faviconICO)
	})
}
