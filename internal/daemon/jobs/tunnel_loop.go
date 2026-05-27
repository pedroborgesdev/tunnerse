package jobs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/config"
	serverembed "github.com/pedroborgesdev/tunnerse-cli/internal/daemon/embed"

	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/logger"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/models"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/utils"
)

type LoopJob struct {
	ID          string
	tunnelURL   string
	localAPIURL string
	isSubdomain bool
	stopChan    chan struct{}
	stopped     bool
	stopMu      sync.Mutex
}

const maxConcurrentTunnelRequests = 128
const maxTunnelBodyBytes = 32 << 20

func (s *LoopJob) Stop() {
	s.stopMu.Lock()
	defer s.stopMu.Unlock()

	if !s.stopped {
		close(s.stopChan)
		s.stopped = true
	}
}

func NewLoopJob(ID string, port string, isSubdomain bool, tunnelURL string) *LoopJob {
	localAPIURL := fmt.Sprintf("http://localhost:%s", port)

	job := &LoopJob{
		ID:          ID,
		tunnelURL:   tunnelURL,
		localAPIURL: localAPIURL,
		isSubdomain: isSubdomain,
		stopChan:    make(chan struct{}),
	}

	return job
}

func (s *LoopJob) SendResponseToServer(data *models.ResponseData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	responseURL := s.tunnelURL + "/response"
	logger.Log("DEBUG", "sending response to server", []logger.LogDetail{
		{Key: "tunnel_id", Value: s.ID},
		{Key: "response_url", Value: responseURL},
	})

	resp, err := serverHTTPClient.Post(responseURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Log("ERROR", "failed to send response", []logger.LogDetail{
			{Key: "tunnel_id", Value: s.ID},
			{Key: "response_url", Value: responseURL},
			{Key: "error", Value: err.Error()},
		})
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server rejected tunnel response: status=%d body=%s", resp.StatusCode, string(body))
	}

	return nil
}

func (s *LoopJob) StartTunnelLoop() {
	if err := logger.SetTunnelLogFile(s.ID, config.LogsDir); err != nil {
		logger.Log("ERROR", "failed to create log file", []logger.LogDetail{
			{Key: "tunnel_id", Value: s.ID},
			{Key: "logs_dir", Value: config.LogsDir},
			{Key: "error", Value: err.Error()},
		})
	}

	defer func() {
		config.RemoveHTTPTunnelURL(s.ID)
		config.RemoveActiveJob(s.ID)
	}()

	workerSlots := make(chan struct{}, maxConcurrentTunnelRequests)
	var workerWG sync.WaitGroup
	var backgroundWG sync.WaitGroup
	defer func() {
		s.Stop()
		workerWG.Wait()
		backgroundWG.Wait()
		logger.CloseTunnelLogFile(s.ID)
	}()

	logger.Log("INFO", "starting tunnel loop", []logger.LogDetail{
		{Key: "tunnel_id", Value: s.ID},
		{Key: "max_concurrent_requests", Value: maxConcurrentTunnelRequests},
	})

	var errorTimestamps []time.Time
	const rateLimitCount = 10
	const rateLimitWindow = 10 * time.Second

	backgroundWG.Add(2)
	go func() {
		defer backgroundWG.Done()
		s.healthcheckLocalAPI()
	}()
	go func() {
		defer backgroundWG.Done()
		s.pingToServer()
	}()

	for {
		select {
		case <-s.stopChan:
			logger.Log("INFO", "tunnel loop stopped by external signal", []logger.LogDetail{
				{Key: "tunnel_id", Value: s.ID},
			})
			return
		default:
		}

		now := time.Now()
		errorTimestamps = filterRecent(errorTimestamps, now, rateLimitWindow)

		if len(errorTimestamps) >= rateLimitCount {
			logger.Log("FATAL", "target server did not respond, the tunnel was closed", []logger.LogDetail{
				{Key: "tunnel_id", Value: s.ID},
			})
			s.Stop()
			return
		}

		reqData, err := s.FetchRequest()

		if reqData != nil && err != nil && err.Error() == "healthcheck-question" {
			respData := &models.ResponseData{
				StatusCode: 204,
				Headers: map[string][]string{
					"Tunnerse": {"healthcheck-conclued"},
				},
				Body:  nil,
				Token: reqData.Token,
			}
			err = s.SendResponseToServer(respData)
			if err != nil {
				logger.Log("ERROR", "failed to send healthcheck response", []logger.LogDetail{
					{Key: "tunnel_id", Value: s.ID},
					{Key: "error", Value: err.Error()},
				})
			}
			continue
		}

		if reqData == nil && err == nil {
			continue
		}

		if err != nil {
			if err.Error() == "tunnel has closed by server" {
				logger.Log("FATAL", "tunnel has closed by server", []logger.LogDetail{
					{Key: "tunnel_id", Value: s.ID},
				})
				s.Stop()
				return
			}
			if err.Error() == "response-time-exceeded" {
				logger.Log("FATAL", "reponse time exceeded, the tunnel was closed", []logger.LogDetail{
					{Key: "tunnel_id", Value: s.ID},
				})
				s.Stop()
				return
			}
			errorTimestamps = append(errorTimestamps, now)
			continue
		}

		select {
		case workerSlots <- struct{}{}:
		case <-s.stopChan:
			continue
		}

		workerWG.Add(1)
		go func(reqData *models.RequestData) {
			defer workerWG.Done()
			defer func() { <-workerSlots }()

			if err := s.processRequest(reqData); err != nil {
				logger.Log("FATAL", "error during tunnel request processing", []logger.LogDetail{
					{Key: "tunnel_id", Value: s.ID},
					{Key: "path", Value: reqData.Path},
					{Key: "error", Value: err.Error()},
				})
				s.Stop()
			}
		}(reqData)
	}
}

func (s *LoopJob) processRequest(reqData *models.RequestData) error {
	respData, err := s.ForwardToLocal(reqData)
	if err != nil {
		logger.Log("WARN", "failed to forward request to local API", []logger.LogDetail{
			{Key: "tunnel_id", Value: s.ID},
			{Key: "path", Value: reqData.Path},
			{Key: "error", Value: err.Error()},
		})

		errorResp := &models.ResponseData{
			StatusCode: http.StatusServiceUnavailable,
			Headers: map[string][]string{
				"Content-Type": {"text/plain; charset=utf-8"},
				"Tunnerse":     {"local-api-error"},
			},
			Token: reqData.Token,
		}

		return s.SendResponseToServer(errorResp)
	}

	return s.SendResponseToServer(respData)
}

func (s *LoopJob) FetchRequest() (*models.RequestData, error) {
	fetchURL := s.tunnelURL + "/tunnel"
	logger.Log("DEBUG", "fetching request from server", []logger.LogDetail{
		{Key: "tunnel_id", Value: s.ID},
		{Key: "fetch_url", Value: fetchURL},
	})

	resp, err := serverHTTPClient.Get(fetchURL)
	if err != nil {
		logger.Log("ERROR", "failed to fetch request", []logger.LogDetail{
			{Key: "tunnel_id", Value: s.ID},
			{Key: "fetch_url", Value: fetchURL},
			{Key: "error", Value: err.Error()},
		})
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusGatewayTimeout:
			return nil, fmt.Errorf("response-time-exceeded")
		default:
			return nil, fmt.Errorf("unexpected response by server")
		}
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxTunnelBodyBytes*2+1))
	if err != nil {
		return nil, err
	}
	if len(bodyBytes) > maxTunnelBodyBytes*2 {
		return nil, fmt.Errorf("tunnel request body too large")
	}

	var requestData models.RequestData

	err = json.Unmarshal(bodyBytes, &requestData)
	if err != nil {
		return nil, fmt.Errorf("unexpected response by server: %s", err.Error())
	}

	value, ok := requestData.Headers["Tunnerse"]
	if ok && len(value) > 0 {
		switch value[0] {
		case "healthcheck-question":
			return &requestData, fmt.Errorf("healthcheck-question")
		case "tunnel-not-found":
			return nil, fmt.Errorf("notfound")
		case "tunnel-timeout":
			return nil, fmt.Errorf("timeout")
		case "tunnel-working":
			return nil, fmt.Errorf("working")
		}
	}

	fmt.Println(requestData.Path)

	return &requestData, nil
}

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

var serverHTTPClient = &http.Client{
	Timeout: 75 * time.Second,
}

func (s *LoopJob) ForwardToLocal(req *models.RequestData) (*models.ResponseData, error) {
	if isTunnerseDemoPath(req.Path) {
		demoResp, err := serveDemoHTML(req.Path)
		if err != nil {
			return nil, err
		}
		demoResp.Token = req.Token
		return demoResp, nil
	}

	path := req.Path
	tunnelPrefix := "/" + s.ID + "/"
	if strings.HasPrefix(path, tunnelPrefix) {
		path = "/" + strings.TrimPrefix(path, tunnelPrefix)
	}
	url := fmt.Sprintf("%s%s", s.localAPIURL, path)

	request, err := http.NewRequest(req.Method, url, bytes.NewBuffer(req.Body))
	if err != nil {
		return nil, err
	}

	for key, values := range req.Headers {
		switch strings.ToLower(key) {
		case "accept-encoding", "connection", "content-length", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
			continue
		}
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	request.Header.Set("Accept-Encoding", "identity")

	resp, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTunnelBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxTunnelBodyBytes {
		return nil, fmt.Errorf("local response body too large")
	}

	contentType := resp.Header.Get("Content-Type")
	if resp.Header.Get("Content-Encoding") == "" {
		if !s.isSubdomain {
			body = utils.RewriteAbsolutePathsForContentType(body, s.ID, contentType)
			body = utils.InjectTunnerseTunnelHeaderForAppDocument(body, path, contentType)
		} else {
			body = utils.InjectTunnerseTunnelHeaderForAppDocument(body, path, contentType)
		}
	}

	headers := make(map[string][]string)
	for key, values := range resp.Header {
		switch strings.ToLower(key) {
		case "connection", "content-length", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
			continue
		}
		headers[key] = values
	}

	if _, ok := headers["Content-Type"]; !ok {
		headers["Content-Type"] = []string{"text/html; charset=utf-8"}
	}

	var respData *models.ResponseData
	respData = &models.ResponseData{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
		Token:      req.Token,
	}

	return respData, nil
}

func isTunnerseDemoPath(path string) bool {
	p := path
	if p == "" {
		return false
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	if p == "/tunnerse" || strings.HasPrefix(p, "/tunnerse/") {
		return true
	}
	return false
}

func serveDemoHTML(requestPath string) (*models.ResponseData, error) {
	headers := map[string][]string{
		"Content-Type": {"text/html; charset=utf-8"},
		"Tunnerse":     {"demo"},
	}

	return &models.ResponseData{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       serverembed.DemoHTML(),
	}, nil
}

func filterRecent(timestamps []time.Time, now time.Time, window time.Duration) []time.Time {
	filtered := timestamps[:0]
	for _, t := range timestamps {
		if now.Sub(t) <= window {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func (s *LoopJob) closeConnection() error {
	payload := map[string]string{"name": s.ID}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}

	resp, err := serverHTTPClient.Post(s.tunnelURL+"/close", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("post register: %w", err)
	}
	defer resp.Body.Close()

	return nil
}
