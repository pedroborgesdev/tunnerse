package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/config"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/debug"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/expose"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/logger"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/middlewares"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/routes"
)

func main() {
	_ = debug.LoadDebugConfig()
	config.LoadAppConfig()

	errCh, err := expose.StartExpose()
	if err != nil {
		fmt.Printf("\nFailed to start expose: %s\n", err.Error())
		os.Exit(1)
	}
	go func() {
		if exposeErr := <-errCh; exposeErr != nil {
			fmt.Printf("\nExpose error: %s\n", exposeErr.Error())
			os.Exit(1)
		}
	}()

	logger.Log("INFO", "Application has been started", []logger.LogDetail{})

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	router.Use(
		middlewares.CORSMiddleware(),
	)

	routes.SetupRoutes(router)

	writeTimeout := time.Duration(config.AppConfig.TUNNEL_REQUEST_TIMEOUT+30) * time.Second
	if writeTimeout < 90*time.Second {
		writeTimeout = 90 * time.Second
	}

	server := &http.Server{
		Addr:              ":" + config.AppConfig.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       90 * time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("\nServer error: %s\n", err.Error())
		os.Exit(1)
	}
}
