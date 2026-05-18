package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/config"
	apiembed "github.com/pedroborgesdev/tunnerse-cli/internal/server/embed"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/logger"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/models"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/utils"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/validation"
)

type TunnelService struct {
	validator *validation.TunnelValidator
	tunnels   map[string]*Tunnel
	mux       sync.RWMutex
}

func NewTunnelService() *TunnelService {
	return &TunnelService{
		validator: validation.NewTunnelValidator(),
		tunnels:   make(map[string]*Tunnel),
	}
}

type Tunnel struct {
	requestCh       chan *http.Request
	writerCh        chan http.ResponseWriter
	pendingRequests map[string]chan *ResponseWithToken
	resetTimer      func()
	stopTimer       chan struct{}
	done            chan struct{}
	closed          bool
	closeOnce       sync.Once
	mu              sync.Mutex
}

type ResponseWithToken struct {
	Writer http.ResponseWriter
	Resp   *models.ResponseData
}

func positiveOrDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

const (
	maxTunnelBodyBytes         = 32 << 20
	maxTunnelResponseJSONBytes = maxTunnelBodyBytes * 2
)

func (s *TunnelService) Register(name string) (string, error) {
	if err := s.validator.ValidateTunnelRegister(name); err != nil {
		return "", err
	}

	var tunnelName string
	for {
		random := utils.RandomCode(8)
		tunnelName = name + "-" + random

		s.mux.RLock()
		_, exists := s.tunnels[tunnelName]
		s.mux.RUnlock()

		if !exists {
			break
		}
	}

	queueSize := positiveOrDefault(config.AppConfig.TUNNEL_QUEUE_SIZE, 256)
	t := &Tunnel{
		requestCh:       make(chan *http.Request, queueSize),
		writerCh:        make(chan http.ResponseWriter),
		pendingRequests: make(map[string]chan *ResponseWithToken),
		stopTimer:       make(chan struct{}),
		done:            make(chan struct{}),
	}

	inactivityDuration := time.Duration(positiveOrDefault(config.AppConfig.TUNNEL_INACTIVITY_LIFE_TIME, 86400)) * time.Second
	inactivityTimer := time.NewTimer(inactivityDuration)

	var maxLifetimeTimer *time.Timer
	maxLifetimeDuration := time.Duration(config.AppConfig.TUNNEL_LIFE_TIME) * time.Second
	hasMaxLifetime := config.AppConfig.TUNNEL_LIFE_TIME > 0
	if hasMaxLifetime {
		maxLifetimeTimer = time.NewTimer(maxLifetimeDuration)
	}

	t.resetTimer = func() {
		if !inactivityTimer.Stop() {
			select {
			case <-inactivityTimer.C:
			default:
			}
		}
		inactivityTimer.Reset(inactivityDuration)
	}

	s.mux.Lock()
	s.tunnels[tunnelName] = t
	s.mux.Unlock()

	logger.Log("DEBUG", "Tunnel registered with bounded queue", []logger.LogDetail{
		{Key: "tunnel", Value: tunnelName},
		{Key: "queue_capacity", Value: cap(t.requestCh)},
		{Key: "max_pending_requests", Value: positiveOrDefault(config.AppConfig.TUNNEL_MAX_PENDING_REQUESTS, cap(t.requestCh))},
	})

	go func(tunnelName string, t *Tunnel) {
		defer func() {
			inactivityTimer.Stop()
			if hasMaxLifetime {
				maxLifetimeTimer.Stop()
			}

			t.mu.Lock()
			t.closed = true

			for token, ch := range t.pendingRequests {
				close(ch)
				delete(t.pendingRequests, token)
			}
			t.closeOnce.Do(func() {
				close(t.done)
			})
			t.mu.Unlock()

			s.mux.Lock()
			delete(s.tunnels, tunnelName)
			s.mux.Unlock()
		}()

		if hasMaxLifetime {
			select {
			case <-inactivityTimer.C:
			case <-maxLifetimeTimer.C:
			case <-t.stopTimer:
			}
		} else {
			select {
			case <-inactivityTimer.C:
			case <-t.stopTimer:
			}
		}
	}(tunnelName, t)

	return tunnelName, nil
}

func (s *TunnelService) Get(name string, r *http.Request) ([]byte, error) {
	s.mux.RLock()
	tunnel, exists := s.tunnels[name]
	s.mux.RUnlock()

	if !exists {
		return nil, fmt.Errorf("tunnel not found")
	}

	tunnel.mu.Lock()
	if tunnel.closed {
		tunnel.mu.Unlock()
		return nil, fmt.Errorf("tunnel is closed")
	}
	if tunnel.resetTimer != nil {
		tunnel.resetTimer()
	}
	done := tunnel.done
	tunnel.mu.Unlock()

	var req *http.Request

	select {
	case req = <-tunnel.requestCh:
		if req == nil {
			return nil, fmt.Errorf("nil request received")
		}
	case <-done:
		return nil, fmt.Errorf("tunnel is closed")
	case <-r.Context().Done():
		return nil, fmt.Errorf("client disconnected; tunnel has a 1-minute grace period")
	}

	tunnel.mu.Lock()
	closed := tunnel.closed
	tunnel.mu.Unlock()
	if closed {
		return nil, fmt.Errorf("tunnel is closed")
	}

	token := req.Header.Get("Tunnerse-Request-Token")
	if token == "" {
		if tokenVal := req.Context().Value("tunnerse-token"); tokenVal != nil {
			token = tokenVal.(string)
		}
	}

	var bodyBytes []byte
	if req.Body != nil {
		defer req.Body.Close()
		var err error
		bodyBytes, err = io.ReadAll(io.LimitReader(req.Body, maxTunnelBodyBytes+1))
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		if len(bodyBytes) > maxTunnelBodyBytes {
			return nil, fmt.Errorf("request body too large")
		}
	}

	headersCopy := make(map[string][]string, len(req.Header))
	for k, v := range req.Header {
		copied := make([]string, len(v))
		copy(copied, v)
		headersCopy[k] = copied
	}

	sreq := models.SerializableRequest{
		Method: req.Method,
		Path:   req.URL.String(),
		Header: headersCopy,
		Body:   base64.StdEncoding.EncodeToString(bodyBytes),
		Host:   req.Host,
		Token:  token,
	}

	return json.Marshal(&sreq)
}

func (s *TunnelService) Response(name string, body io.ReadCloser) error {
	defer body.Close()

	s.mux.RLock()
	tunnel, exists := s.tunnels[name]
	s.mux.RUnlock()
	if !exists {
		return fmt.Errorf("tunnel not found")
	}

	rawBody, err := io.ReadAll(io.LimitReader(body, maxTunnelResponseJSONBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read response JSON: %w", err)
	}
	if len(rawBody) > maxTunnelResponseJSONBytes {
		return fmt.Errorf("response body too large")
	}

	var resp models.ResponseData
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		return fmt.Errorf("failed to decode response JSON: %w", err)
	}

	logger.Log("DEBUG", "Response headers received", []logger.LogDetail{
		{Key: "tunnel", Value: name},
		{Key: "token", Value: resp.Token},
		{Key: "headers", Value: fmt.Sprintf("%+v", resp.Headers)},
	})

	if resp.Token == "" {
		return fmt.Errorf("missing Tunnerse-Request-Token in response")
	}
	if base64.StdEncoding.DecodedLen(len(resp.Body)) > maxTunnelBodyBytes {
		return fmt.Errorf("response body too large")
	}

	tunnel.mu.Lock()
	if tunnel.closed {
		tunnel.mu.Unlock()
		return fmt.Errorf("tunnel is closed")
	}

	responseCh, exists := tunnel.pendingRequests[resp.Token]
	if !exists {
		tunnel.mu.Unlock()
		return fmt.Errorf("no pending request found for token: %s (expired or invalid)", resp.Token)
	}
	delete(tunnel.pendingRequests, resp.Token)
	tunnel.mu.Unlock()

	select {
	case responseCh <- &ResponseWithToken{Resp: &resp}:
		close(responseCh)
	case <-time.After(5 * time.Second):
		close(responseCh)
		return fmt.Errorf("response channel timeout for token: %s", resp.Token)
	}

	return nil
}

func (s *TunnelService) Tunnel(name, path string, w http.ResponseWriter, r *http.Request) error {
	if err := s.validator.ValidateTunnelRegister(name); err != nil {
		return err
	}

	s.mux.RLock()
	tunnel, exists := s.tunnels[name]
	s.mux.RUnlock()
	if !exists {
		return fmt.Errorf("tunnel not found")
	}

	tunnel.mu.Lock()
	if tunnel.closed {
		tunnel.mu.Unlock()
		return fmt.Errorf("tunnel is closed")
	}
	if tunnel.resetTimer != nil {
		tunnel.resetTimer()
	}
	done := tunnel.done
	tunnel.mu.Unlock()

	token := uuid.New().String()

	responseCh := make(chan *ResponseWithToken, 1)

	tunnel.mu.Lock()
	if tunnel.closed {
		tunnel.mu.Unlock()
		return fmt.Errorf("tunnel is closed")
	}
	maxPendingRequests := positiveOrDefault(config.AppConfig.TUNNEL_MAX_PENDING_REQUESTS, cap(tunnel.requestCh))
	if len(tunnel.pendingRequests) >= maxPendingRequests {
		tunnel.mu.Unlock()
		logger.Log("WARN", "Tunnel pending request limit reached", []logger.LogDetail{
			{Key: "tunnel", Value: name},
			{Key: "pending_requests", Value: maxPendingRequests},
		})
		return fmt.Errorf("timeout")
	}
	tunnel.pendingRequests[token] = responseCh
	tunnel.mu.Unlock()

	defer func() {
		tunnel.mu.Lock()
		delete(tunnel.pendingRequests, token)
		tunnel.mu.Unlock()
	}()

	var bodyBytes []byte
	if r.Body != nil {
		defer r.Body.Close()
		var err error
		bodyBytes, err = io.ReadAll(io.LimitReader(r.Body, maxTunnelBodyBytes+1))
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}
		if len(bodyBytes) > maxTunnelBodyBytes {
			return fmt.Errorf("request body too large")
		}
	}

	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	clonedRequest := r.Clone(r.Context())
	clonedRequest.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	clonedRequest.Header.Set("Tunnerse-Request-Token", token)

	if !config.AppConfig.SUBDOMAIN {
		if parts := strings.SplitN(clonedRequest.URL.Path, "/", 3); len(parts) >= 3 {
			clonedRequest.URL.Path = "/" + parts[2]
		} else {
			clonedRequest.URL.Path = "/"
		}
		clonedRequest.RequestURI = clonedRequest.URL.RequestURI()
	} else {
		clonedRequest.URL.Path = path
		clonedRequest.RequestURI = clonedRequest.URL.RequestURI()
	}

	timeout := time.Duration(positiveOrDefault(config.AppConfig.TUNNEL_REQUEST_TIMEOUT, 30)) * time.Second

	tunnel.mu.Lock()
	if tunnel.closed {
		tunnel.mu.Unlock()
		return fmt.Errorf("tunnel is closed")
	}
	requestCh := tunnel.requestCh
	done = tunnel.done
	tunnel.mu.Unlock()

	select {
	case requestCh <- clonedRequest:
	case <-done:
		return fmt.Errorf("tunnel is closed")
	case <-time.After(timeout):
		return fmt.Errorf("timeout")
	case <-r.Context().Done():
		return fmt.Errorf("client disconnected")
	}

	select {
	case respData := <-responseCh:
		if respData == nil || respData.Resp == nil {
			return fmt.Errorf("received nil response")
		}

		if tunnerseHeader, ok := respData.Resp.Headers["Tunnerse"]; ok && len(tunnerseHeader) > 0 {
			if tunnerseHeader[0] == "local-api-error" {
				return fmt.Errorf("local-api-error")
			}
		}

		bodyDecoded, err := base64.StdEncoding.DecodeString(respData.Resp.Body)
		if err != nil {
			return fmt.Errorf("failed to decode base64 body: %w", err)
		}

		for key, values := range respData.Resp.Headers {
			switch strings.ToLower(key) {
			case "connection", "content-length", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
				continue
			}
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}

		w.WriteHeader(respData.Resp.StatusCode)
		_, err = w.Write(bodyDecoded)
		return err

	case <-done:
		return fmt.Errorf("tunnel is closed")
	case <-time.After(timeout):
		return fmt.Errorf("timeout")
	case <-r.Context().Done():
		return fmt.Errorf("client disconnected")
	}
}

func (s *TunnelService) Close(name string) error {
	s.mux.Lock()
	tunnel, exists := s.tunnels[name]
	if !exists {
		s.mux.Unlock()
		return fmt.Errorf("tunnel not found")
	}
	delete(s.tunnels, name)
	s.mux.Unlock()

	tunnel.mu.Lock()
	alreadyClosed := tunnel.closed
	tunnel.closed = true
	for token, ch := range tunnel.pendingRequests {
		close(ch)
		delete(tunnel.pendingRequests, token)
	}
	tunnel.closeOnce.Do(func() {
		close(tunnel.done)
	})
	tunnel.mu.Unlock()

	if alreadyClosed {
		return nil
	}

	select {
	case tunnel.stopTimer <- struct{}{}:
	default:
	}

	return nil
}

func (s *TunnelService) serveHTML(w http.ResponseWriter, status int, headerValue, folder, fallbackMsg string) {
	data, err := apiembed.HTML(folder)
	if err != nil {
		http.Error(w, fallbackMsg, status)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Tunnerse", headerValue)
	w.WriteHeader(status)

	w.Write(data)
}

func (s *TunnelService) NotFound(w http.ResponseWriter) {
	s.serveHTML(w, http.StatusNotFound, "tunnel-not-found", "notfound", "404 - tunnel not found")
}

func (s *TunnelService) Timeout(w http.ResponseWriter) {
	s.serveHTML(w, http.StatusRequestTimeout, "tunnel-timeout", "timeout", "408 - tunnel timeout")
}

func (s *TunnelService) LocalError(w http.ResponseWriter) {
	s.serveHTML(w, http.StatusServiceUnavailable, "local-api-error", "localerror", "503 - local api error")
}

func (s *TunnelService) Home(w http.ResponseWriter) {
	s.serveHTML(w, http.StatusOK, "tunnel-working", "running", "Tunnerse is running")
}
