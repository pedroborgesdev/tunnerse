package config

import (
	"sync"
)

var (
	tunnelID     = "undefined"
	serverURL    = "localhost:3000"
	is_subdomain = false
	mu           sync.RWMutex
)

var HTTPTunnelURLs = map[string]string{}
var httpTunnelURLsMu sync.RWMutex
var TCPTunnelEndpoints = map[string]string{}
var tcpTunnelEndpointsMu sync.RWMutex

func SetHTTPTunnelURL(tunnelID, tunnelURL string) {
	httpTunnelURLsMu.Lock()
	defer httpTunnelURLsMu.Unlock()
	HTTPTunnelURLs[tunnelID] = tunnelURL
}

func GetHTTPTunnelURL(tunnelID string) (string, bool) {
	httpTunnelURLsMu.RLock()
	defer httpTunnelURLsMu.RUnlock()
	tunnelURL, exists := HTTPTunnelURLs[tunnelID]
	return tunnelURL, exists
}

func RemoveHTTPTunnelURL(tunnelID string) {
	httpTunnelURLsMu.Lock()
	defer httpTunnelURLsMu.Unlock()
	delete(HTTPTunnelURLs, tunnelID)
}

func SetTCPTunnelEndpoint(tunnelID, endpoint string) {
	tcpTunnelEndpointsMu.Lock()
	defer tcpTunnelEndpointsMu.Unlock()
	TCPTunnelEndpoints[tunnelID] = endpoint
}

func GetTCPTunnelEndpoint(tunnelID string) (string, bool) {
	tcpTunnelEndpointsMu.RLock()
	defer tcpTunnelEndpointsMu.RUnlock()
	endpoint, exists := TCPTunnelEndpoints[tunnelID]
	return endpoint, exists
}

func RemoveTCPTunnelEndpoint(tunnelID string) {
	tcpTunnelEndpointsMu.Lock()
	defer tcpTunnelEndpointsMu.Unlock()
	delete(TCPTunnelEndpoints, tunnelID)
}

type TunnelJob interface {
	Stop()
}

var ActiveJobs = map[string]TunnelJob{}
var jobsMu sync.RWMutex

func SetActiveJob(tunnelID string, job TunnelJob) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	ActiveJobs[tunnelID] = job
}

func GetActiveJob(tunnelID string) (TunnelJob, bool) {
	jobsMu.RLock()
	defer jobsMu.RUnlock()
	job, exists := ActiveJobs[tunnelID]
	return job, exists
}

func RemoveActiveJob(tunnelID string) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	delete(ActiveJobs, tunnelID)
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

func GetTunnelHTTPSURL() string {
	mu.RLock()
	defer mu.RUnlock()
	if GetSubdomainBool() {
		return "https://" + tunnelID + "." + serverURL
	}
	return "https://" + serverURL + "/" + tunnelID
}
