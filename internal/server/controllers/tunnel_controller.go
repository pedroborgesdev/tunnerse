package controllers

import (
	"net/http"

	"github.com/pedroborgesdev/tunnerse-cli/internal/server/config"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/logger"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/services"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/utils"

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

func (c *TunnelController) respondNoTunnel(ctx *gin.Context) {
	if config.AppConfig.WARNS_ON_HTML {
		c.tunnelService.Home(ctx.Writer)
		return
	}
	utils.Success(ctx, gin.H{"message": "Tunnerse is running :)"})
}

func (c *TunnelController) Register(ctx *gin.Context) {
	var req utils.RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		return
	}

	tunnelName, err := c.tunnelService.Register(req.Name)
	if err != nil {
		if config.AppConfig.WARNS_ON_HTML && err.Error() == "tunnel not found" {
			c.tunnelService.NotFound(ctx.Writer)
			return
		}
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		logger.Log("ERROR", "Registration failed", []logger.LogDetail{{Key: "Error", Value: err.Error()}})
		return
	}

	utils.Success(ctx, gin.H{
		"message":   "tunnel has been registered",
		"subdomain": config.AppConfig.SUBDOMAIN,
		"tunnel":    tunnelName,
	})
	logger.Log("INFO", "User registered successfully", []logger.LogDetail{
		{Key: "subdomain", Value: config.AppConfig.SUBDOMAIN},
		{Key: "tunnel", Value: tunnelName},
	})
}

func (c *TunnelController) Get(ctx *gin.Context) {
	name := utils.GetTunnelName(ctx)
	if name == "" {
		c.respondNoTunnel(ctx)
		return
	}

	body, err := c.tunnelService.Get(name, ctx.Request)
	if err != nil {
		if config.AppConfig.WARNS_ON_HTML && err.Error() == "tunnel not found" {
			c.tunnelService.NotFound(ctx.Writer)
			return
		}
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		logger.Log("ERROR", "Tunneling failed", []logger.LogDetail{{Key: "Error", Value: err.Error()}})
		return
	}

	ctx.Writer.Write(body)
	logger.Log("INFO", "Message has been written", []logger.LogDetail{{Key: "tunnel", Value: name}})
}

func (c *TunnelController) Response(ctx *gin.Context) {
	name := utils.GetTunnelName(ctx)
	if name == "" {
		c.respondNoTunnel(ctx)
		return
	}

	err := c.tunnelService.Response(name, ctx.Request.Body)
	if err != nil {
		if config.AppConfig.WARNS_ON_HTML && err.Error() == "tunnel not found" {
			c.tunnelService.NotFound(ctx.Writer)
			return
		}
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		logger.Log("ERROR", "Tunneling failed", []logger.LogDetail{{Key: "Error", Value: err.Error()}})
		return
	}

	ctx.Writer.WriteHeader(http.StatusOK)
	logger.Log("INFO", "Message has been written", []logger.LogDetail{{Key: "tunnel", Value: name}})
}

func (c *TunnelController) Tunnel(ctx *gin.Context) {
	name := utils.GetTunnelName(ctx)
	if name == "" {
		c.respondNoTunnel(ctx)
		return
	}

	err := c.tunnelService.Tunnel(name, ctx.Request.URL.Path, ctx.Writer, ctx.Request)
	if err != nil {
		if config.AppConfig.WARNS_ON_HTML {
			switch err.Error() {
			case "tunnel not found":
				c.tunnelService.NotFound(ctx.Writer)
			case "timeout":
				c.tunnelService.Timeout(ctx.Writer)
			case "local-api-error":
				c.tunnelService.LocalError(ctx.Writer)
			default:
				ctx.String(http.StatusInternalServerError, err.Error())
			}
			return
		}
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		logger.Log("ERROR", "Tunneling failed", []logger.LogDetail{{Key: "Error", Value: err.Error()}})
		return
	}

	logger.Log("INFO", "Message has been written", []logger.LogDetail{
		{Key: "tunnel", Value: name},
		{Key: "path", Value: ctx.Request.URL.Path},
	})
}

func (c *TunnelController) Close(ctx *gin.Context) {
	name := utils.GetTunnelName(ctx)
	if name == "" {
		c.respondNoTunnel(ctx)
		return
	}

	err := c.tunnelService.Close(name)
	if err != nil {
		if config.AppConfig.WARNS_ON_HTML && err.Error() == "tunnel not found" {
			c.tunnelService.NotFound(ctx.Writer)
			return
		}
		utils.BadRequest(ctx, gin.H{"error": err.Error()})
		logger.Log("ERROR", "Failed to delete tunnel", []logger.LogDetail{{Key: "Error", Value: err.Error()}})
		return
	}

	logger.Log("INFO", "Tunnel has been deleted", []logger.LogDetail{{Key: "tunnel", Value: name}})
}
