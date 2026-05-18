package utils

type RegisterRequest struct {
	Name string `json:"name" binding:"required"`
}

type OpenRequest struct {
	Name      string `json:"name" binding:"required"`
	Port      string `json:"port" binding:"required"`
	ServerURL string `json:"server_url" binding:"required"`
}

type StopHTTPRequest struct {
	TunnelID string `json:"tunnel_id" binding:"required"`
}
