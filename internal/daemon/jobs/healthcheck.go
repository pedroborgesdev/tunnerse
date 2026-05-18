package jobs

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/logger"
)

func (s *LoopJob) healthcheckLocalAPI() {

	select {
	case <-s.stopChan:
		return
	case <-time.After(5 * time.Second):

	}

	failCount := 0
	maxFails := 10

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			logger.Log("INFO", "healthcheck stopped", []logger.LogDetail{
				{Key: "tunnel_id", Value: s.ID},
			})
			return
		case <-ticker.C:
			resp, err := httpClient.Get(s.localAPIURL)
			if err != nil {
				failCount++
				if isConnectionRefused(err) {
					logger.Log("WARN", "local API connection refused", []logger.LogDetail{
						{Key: "tunnel_id", Value: s.ID},
						{Key: "attempt", Value: fmt.Sprintf("%d", failCount)},
					})
				} else {
					logger.Log("WARN", "health check failed", []logger.LogDetail{
						{Key: "tunnel_id", Value: s.ID},
						{Key: "attempt", Value: fmt.Sprintf("%d", failCount)},
						{Key: "error", Value: err.Error()},
					})
				}

				if failCount >= maxFails {
					logger.Log("FATAL", fmt.Sprintf("local API failed %d times. closing tunnel.", maxFails), []logger.LogDetail{
						{Key: "tunnel_id", Value: s.ID},
					})
					err := s.closeConnection()
					if err != nil {
						logger.Log("FATAL", "error to close tunnel", []logger.LogDetail{
							{Key: "tunnel_id", Value: s.ID},
						})
					}
					s.Stop()
					return
				}
			} else {
				resp.Body.Close()
				if failCount > 0 {
					logger.Log("INFO", "local API reestablished", []logger.LogDetail{
						{Key: "tunnel_id", Value: s.ID},
					})
				}
				failCount = 0
			}
		}
	}
}

func (s *LoopJob) pingToServer() {

	select {
	case <-s.stopChan:
		return
	case <-time.After(5 * time.Second):

	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			logger.Log("INFO", "ping to server stopped", []logger.LogDetail{
				{Key: "tunnel_id", Value: s.ID},
			})
			return
		case <-ticker.C:
			s.sendPing()
		}
	}
}

func (s *LoopJob) sendPing() {
	url := strings.TrimRight(s.tunnelURL, "/") + "/_tunnerse_healthcheck"
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		logger.Log("ERROR", "error during send healthcheck challenge", []logger.LogDetail{
			{Key: "tunnel_id", Value: s.ID},
			{Key: "error", Value: err.Error()},
		})
		return
	}

	req.Header.Set("Tunnerse", "healthcheck-question")

	resp, err := serverHTTPClient.Do(req)
	if err != nil {
		logger.Log("ERROR", "error during process healthcheck challenge", []logger.LogDetail{
			{Key: "tunnel_id", Value: s.ID},
			{Key: "error", Value: err.Error()},
		})
		return
	}
	defer resp.Body.Close()

	if resp.Header.Get("Tunnerse") == "healthcheck-conclued" {
		logger.Log("HEALTHCHECK", "challenge has been overcome", []logger.LogDetail{
			{Key: "tunnel_id", Value: s.ID},
		})
	} else {
		logger.Log("ERROR", "healthcheck challenge has failed", []logger.LogDetail{
			{Key: "tunnel_id", Value: s.ID},
		})
	}
}

func isConnectionRefused(err error) bool {
	return strings.Contains(err.Error(), "connection refused")
}
