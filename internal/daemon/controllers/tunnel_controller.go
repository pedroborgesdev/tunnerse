package controllers

import (
	"os"
	"strconv"

	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/logger"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/services"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/utils"

	"github.com/gin-gonic/gin"
)

type TunnelController struct {
	tunnelService *services.TunnelService
}

func NewTunnelController() *TunnelController {
	return &TunnelController{
		tunnelService: services.NewTunnelService(),
	}
}

func (c *TunnelController) HTTP(ctx *gin.Context) {
	var req utils.OpenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		return
	}

	tunnelName, isSubdomain, err := c.tunnelService.RegisterTunnel(req.Name, req.Port, req.ServerURL)
	if err != nil {
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		logger.Log("ERROR", "HTTP tunnel registration failed", []logger.LogDetail{{Key: "Error", Value: err.Error()}})
		return
	}

	utils.Success(ctx, gin.H{
		"message":   "HTTP tunnel has been registered",
		"subdomain": isSubdomain,
		"tunnel":    tunnelName,
	})
	logger.Log("INFO", "HTTP tunnel registered successfully", []logger.LogDetail{
		{Key: "subdomain", Value: isSubdomain},
		{Key: "tunnel", Value: tunnelName},
	})
}

func (c *TunnelController) StopHTTP(ctx *gin.Context) {
	var req utils.StopHTTPRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		return
	}

	err := c.tunnelService.StopHTTPTunnel(req.TunnelID)
	if err != nil {
		utils.InternalError(ctx, gin.H{"error": err.Error(), "tunnel_id": req.TunnelID})
		logger.Log("ERROR", "Failed to stop HTTP tunnel", []logger.LogDetail{{Key: "Error", Value: err.Error()}, {Key: "tunnel_id", Value: req.TunnelID}})
		return
	}

	utils.Success(ctx, gin.H{
		"message":   "HTTP tunnel has been stopped",
		"tunnel_id": req.TunnelID,
	})
	logger.Log("INFO", "HTTP tunnel stopped successfully", []logger.LogDetail{{Key: "tunnel_id", Value: req.TunnelID}})
}

func (c *TunnelController) TCP(ctx *gin.Context) {
	var req utils.OpenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		return
	}

	tunnelID, endpoint, err := c.tunnelService.RegisterTCPTunnel(req.Name, req.Port, req.ServerURL)
	if err != nil {
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		logger.Log("ERROR", "TCP tunnel registration failed", []logger.LogDetail{{Key: "Error", Value: err.Error()}})
		return
	}

	utils.Success(ctx, gin.H{
		"message":  "TCP tunnel has been registered",
		"tunnel":   tunnelID,
		"endpoint": endpoint,
	})
	logger.Log("INFO", "TCP tunnel registered successfully", []logger.LogDetail{
		{Key: "tunnel", Value: tunnelID},
		{Key: "endpoint", Value: endpoint},
	})
}

func (c *TunnelController) StopTCP(ctx *gin.Context) {
	var req utils.StopTCPRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		return
	}

	err := c.tunnelService.StopTCPTunnel(req.TunnelID)
	if err != nil {
		utils.InternalError(ctx, gin.H{"error": err.Error(), "tunnel_id": req.TunnelID})
		logger.Log("ERROR", "Failed to stop TCP tunnel", []logger.LogDetail{{Key: "Error", Value: err.Error()}, {Key: "tunnel_id", Value: req.TunnelID}})
		return
	}

	utils.Success(ctx, gin.H{
		"message":   "TCP tunnel has been stopped",
		"tunnel_id": req.TunnelID,
	})
	logger.Log("INFO", "TCP tunnel stopped successfully", []logger.LogDetail{{Key: "tunnel_id", Value: req.TunnelID}})
}

func (c *TunnelController) TCPHealth(ctx *gin.Context) {
	tunnelID := ctx.Param("tunnel_id")
	if tunnelID == "" {
		utils.BadRequest(ctx, gin.H{"error": "tunnel_id is required"})
		return
	}

	if err := c.tunnelService.KeepTCPTunnelAlive(tunnelID); err != nil {
		utils.NotFound(ctx, gin.H{"error": err.Error(), "tunnel_id": tunnelID})
		return
	}

	utils.Success(ctx, gin.H{
		"message":   "TCP tunnel heartbeat received",
		"tunnel_id": tunnelID,
	})
}

func (c *TunnelController) Health(ctx *gin.Context) {
	tunnelID := ctx.Param("tunnel_id")
	if tunnelID == "" {
		utils.BadRequest(ctx, gin.H{"error": "tunnel_id is required"})
		return
	}

	if err := c.tunnelService.KeepHTTPTunnelAlive(tunnelID); err != nil {
		utils.NotFound(ctx, gin.H{"error": err.Error(), "tunnel_id": tunnelID})
		return
	}

	utils.Success(ctx, gin.H{
		"message":   "HTTP tunnel heartbeat received",
		"tunnel_id": tunnelID,
	})
}

func (c *TunnelController) Logs(ctx *gin.Context) {
	tunnelID := ctx.Param("tunnel_id")

	offset := int64(0)
	if rawOffset := ctx.Query("offset"); rawOffset != "" {
		parsedOffset, err := strconv.ParseInt(rawOffset, 10, 64)
		if err != nil {
			utils.BadRequest(ctx, gin.H{"error": "invalid offset"})
			return
		}
		offset = parsedOffset
	}

	logs, nextOffset, err := logger.ReadTunnelLog(tunnelID, offset)
	if err != nil {
		if os.IsNotExist(err) {
			utils.NotFound(ctx, gin.H{"error": "log file not found", "tunnel_id": tunnelID})
			return
		}
		utils.BadRequest(ctx, gin.H{"error": err.Error(), "tunnel_id": tunnelID})
		return
	}

	utils.Success(ctx, gin.H{
		"tunnel_id": tunnelID,
		"logs":      logs,
		"offset":    nextOffset,
	})
}
