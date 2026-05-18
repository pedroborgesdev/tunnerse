package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/config"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/logger"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/routes"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := runEntrypoint(); err != nil {
		logger.Log("FATAL", "Application stopped with error", []logger.LogDetail{{Key: "error", Value: err.Error()}})
	}
}

func runServer(ctx context.Context) error {
	if err := config.LoadAppConfig(); err != nil {
		return err
	}

	logger.Log("INFO", "Application has been started", []logger.LogDetail{})

	logger.Log("INFO", "Data directory", []logger.LogDetail{
		{Key: "path", Value: config.GetUserDataDir()},
		{Key: "logs", Value: config.GetLogsDir()},
	})

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	routes.SetupRoutes(router)

	server := &http.Server{
		Addr:              ":" + "9988",
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		if err := server.Shutdown(context.Background()); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
