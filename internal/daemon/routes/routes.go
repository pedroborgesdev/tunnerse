package routes

import (
	"net/http"

	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/controllers"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(router *gin.Engine) {
	tunnelController := controllers.NewTunnelController()

	router.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	tunnel := router.Group("/")

	tunnel.POST("/http", tunnelController.HTTP)
	tunnel.GET("/http/logs/:tunnel_id", tunnelController.Logs)
	tunnel.POST("/http/stop", tunnelController.StopHTTP)
	tunnel.GET("/health/:tunnel_id", tunnelController.Health)
	tunnel.POST("/tcp", tunnelController.TCP)
	tunnel.POST("/tcp/stop", tunnelController.StopTCP)
	tunnel.GET("/tcp/health/:tunnel_id", tunnelController.TCPHealth)
}
