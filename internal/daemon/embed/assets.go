package serverembed

import (
	"encoding/base64"
	"strings"
	"sync"

	_ "embed"
)

//go:embed header.html
var headerHTML string

//go:embed demo.html
var demoHTML []byte

//go:embed icon.png
var iconPNG []byte

//go:embed favicon.ico
var faviconICO []byte

var (
	assetDataOnce sync.Once
	iconPNGData   string
	faviconData   string
)

func HeaderHTML() string {
	return strings.ReplaceAll(headerHTML, "icon.png", IconPNGDataURI())
}

func DemoHTML() []byte {
	html := string(demoHTML)
	html = strings.ReplaceAll(html, "icon.png", IconPNGDataURI())
	html = strings.ReplaceAll(html, "favicon.ico", FaviconDataURI())
	return []byte(html)
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
