package routes

import (
	"net/http"

	"github.com/pedroborgesdev/tunnerse-cli/internal/server/config"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/controllers"
	apiembed "github.com/pedroborgesdev/tunnerse-cli/internal/server/embed"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(router *gin.Engine) {

	tunnelController := controllers.NewTunnelController()

	router.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	router.GET("/favicon.ico", func(c *gin.Context) {
		c.Data(http.StatusOK, "image/x-icon", apiembed.FaviconICO())
	})
	router.HEAD("/favicon.ico", func(c *gin.Context) {
		c.Header("Content-Type", "image/x-icon")
		c.Status(http.StatusOK)
	})

	router.GET("/favicon.ico/", func(c *gin.Context) {
		c.Data(http.StatusOK, "image/x-icon", apiembed.FaviconICO())
	})
	router.HEAD("/favicon.ico/", func(c *gin.Context) {
		c.Header("Content-Type", "image/x-icon")
		c.Status(http.StatusOK)
	})

	tunnel := router.Group("/")

	if config.AppConfig.SUBDOMAIN {
		tunnel.POST("/register", tunnelController.Register)
		tunnel.GET("/tunnel", tunnelController.Get)
		tunnel.POST("/response", tunnelController.Response)
		tunnel.POST("/close", tunnelController.Close)
		tunnel.GET("/", tunnelController.Tunnel)
		tunnel.HEAD("/_tunnerse_healthcheck", tunnelController.Tunnel)

		router.NoRoute(tunnelController.Tunnel)
	}

	if !config.AppConfig.SUBDOMAIN {
		tunnel.POST("/register", tunnelController.Register)
		tunnel.GET(":name/tunnel", tunnelController.Get)
		tunnel.POST(":name/response", tunnelController.Response)
		tunnel.POST(":name/close", tunnelController.Close)
		tunnel.GET(":name/", tunnelController.Tunnel)
		tunnel.HEAD(":name/_tunnerse_healthcheck", tunnelController.Tunnel)

		router.NoRoute(tunnelController.Tunnel)
	}
}
