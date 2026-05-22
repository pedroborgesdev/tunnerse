package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/services"
)

type TCPTunnelController struct {
	tcpTunnelService *services.TCPTunnelService
}

func NewTCPTunnelController(tcpTunnelService *services.TCPTunnelService) *TCPTunnelController {
	return &TCPTunnelController{tcpTunnelService: tcpTunnelService}
}

func (c *TCPTunnelController) WebSocket(ctx *gin.Context) {
	c.tcpTunnelService.HandleWebSocket(ctx.Writer, ctx.Request)
}
