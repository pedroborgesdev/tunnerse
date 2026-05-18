package variables

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	tunnelID     = "undefined"
	serverURL    = "localhost:3000"
	addressURL   = "127.0.0.1:5500"
	tunnelsPath  = "./"
	is_subdomain = false
	mu           sync.RWMutex
)

func SetExecPath() {
	execPath, _ := os.Executable()

	execDir := filepath.Dir(execPath)

	tunnelsPath = filepath.Join(execDir, "tunnels")

	os.MkdirAll(tunnelsPath, 0755)
}

func SetSubdomainBool(subdomain bool) {
	mu.Lock()
	defer mu.Unlock()
	is_subdomain = subdomain
}

func SetTunnelID(id string) {
	mu.Lock()
	defer mu.Unlock()
	tunnelID = id
}

func SetServerURL(url string) {
	mu.Lock()
	defer mu.Unlock()
	serverURL = url
}

func SetAddressURL(address string) {
	mu.Lock()
	defer mu.Unlock()
	addressURL = "http://127.0.0.1:" + address
}

func GetExecPath() string {
	mu.RLock()
	defer mu.RUnlock()
	return tunnelsPath
}

func GetSubdomainBool() bool {
	mu.RLock()
	defer mu.RUnlock()
	return is_subdomain
}

func GetTunnelID() string {
	mu.RLock()
	defer mu.RUnlock()
	return tunnelID
}

func GetServerURL() string {
	mu.RLock()
	defer mu.RUnlock()
	return serverURL
}

func GetTunnelHTTPURL() string {
	mu.RLock()
	defer mu.RUnlock()
	if GetSubdomainBool() {
		return "http://" + tunnelID + "." + serverURL
	}
	return "http://" + serverURL + "/" + tunnelID
}

func GetTunnelHTTPSURL() string {
	mu.RLock()
	defer mu.RUnlock()
	if GetSubdomainBool() {
		return "https://" + tunnelID + "." + serverURL
	}
	return "https://" + serverURL + "/" + tunnelID
}

func GetAddressURL() string {
	mu.RLock()
	defer mu.RUnlock()
	return addressURL
}
