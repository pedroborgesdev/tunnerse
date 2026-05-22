package services

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/logger"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/utils"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/validation"
)

const (
	tcpHeartbeatInterval = 30 * time.Second
	tcpHeartbeatTimeout  = 90 * time.Second
	tcpMaxConnections    = 100
	tcpMaxFramePayload   = 64 << 10
	tcpMaxConnBuffer     = 1 << 20
	tcpReadBufferSize    = 32 << 10
	tcpWriteQueueSize    = 256
)

type TCPTunnelService struct {
	validator         *validation.TunnelValidator
	tunnelsByHostname map[string]*TCPTunnel
	mux               sync.RWMutex
}

type TCPTunnel struct {
	Name        string
	Hostname    string
	WebSocket   *websocket.Conn
	Connections map[uint64]*TCPConnection
	LastPong    time.Time

	writeChan chan tcpWSMessage
	done      chan struct{}
	closeOnce sync.Once
	nextID    uint64
	mu        sync.Mutex
}

type TCPConnection struct {
	ID            uint64
	Conn          net.Conn
	Created       time.Time
	BufferedBytes int
}

type tcpWSMessage struct {
	messageType   int
	connectionID  uint64
	bufferedBytes int
	payload       []byte
}

type tcpControlMessage struct {
	Type         string `json:"type"`
	TunnelName   string `json:"tunnel_name,omitempty"`
	LocalPort    int    `json:"local_port,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
	ConnectionID uint64 `json:"connection_id,omitempty"`
}

var tcpUpgrader = websocket.Upgrader{
	ReadBufferSize:  tcpReadBufferSize,
	WriteBufferSize: tcpReadBufferSize,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewTCPTunnelService() *TCPTunnelService {
	return &TCPTunnelService{
		validator:         validation.NewTunnelValidator(),
		tunnelsByHostname: make(map[string]*TCPTunnel),
	}
}

func (s *TCPTunnelService) IsTCPTunnelHostname(hostname string) bool {
	hostname = normalizeTCPHostname(hostname)
	if hostname == "" {
		return false
	}

	s.mux.RLock()
	_, exists := s.tunnelsByHostname[hostname]
	s.mux.RUnlock()
	return exists
}

func (s *TCPTunnelService) HandleExternalConnection(hostname string, conn net.Conn) {
	hostname = normalizeTCPHostname(hostname)

	s.mux.RLock()
	tunnel, exists := s.tunnelsByHostname[hostname]
	s.mux.RUnlock()
	if !exists {
		conn.Close()
		return
	}

	connectionID, err := tunnel.addConnection(conn)
	if err != nil {
		logger.Log("WARN", "TCP tunnel rejected connection", []logger.LogDetail{
			{Key: "hostname", Value: hostname},
			{Key: "error", Value: err.Error()},
		})
		conn.Close()
		return
	}

	if !tunnel.sendControl(tcpControlMessage{Type: "new_connection", ConnectionID: connectionID}) {
		s.removeTCPConnection(tunnel, connectionID, true)
		return
	}

	logger.Log("INFO", "TCP connection opened", []logger.LogDetail{
		{Key: "hostname", Value: hostname},
		{Key: "connection_id", Value: connectionID},
	})

	go s.pipeTCPToWebSocket(tunnel, connectionID, conn)
}

func (s *TCPTunnelService) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := tcpUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Log("ERROR", "TCP websocket upgrade failed", []logger.LogDetail{{Key: "error", Value: err.Error()}})
		return
	}

	ws.SetReadLimit(tcpMaxFramePayload + 8)
	messageType, payload, err := ws.ReadMessage()
	if err != nil {
		ws.Close()
		return
	}
	if messageType != websocket.TextMessage {
		ws.Close()
		return
	}

	var register tcpControlMessage
	if err := json.Unmarshal(payload, &register); err != nil || register.Type != "register" {
		ws.Close()
		return
	}

	tunnel, err := s.registerTunnel(register.TunnelName, register.LocalPort, tcpHostnameSuffix(r.Host), ws)
	if err != nil {
		_ = ws.WriteJSON(tcpControlMessage{Type: "close"})
		ws.Close()
		logger.Log("ERROR", "TCP tunnel registration failed", []logger.LogDetail{{Key: "error", Value: err.Error()}})
		return
	}

	go tunnel.writeLoop()
	tunnel.sendControl(tcpControlMessage{Type: "registered", Hostname: tunnel.Hostname})

	logger.Log("INFO", "TCP tunnel registered", []logger.LogDetail{
		{Key: "name", Value: tunnel.Name},
		{Key: "hostname", Value: tunnel.Hostname},
	})

	go s.heartbeatLoop(tunnel)
	s.readLoop(tunnel)
	s.unregisterTunnel(tunnel)
}

func (s *TCPTunnelService) registerTunnel(name string, localPort int, hostSuffix string, ws *websocket.Conn) (*TCPTunnel, error) {
	if err := s.validator.ValidateTunnelRegister(name); err != nil {
		return nil, err
	}
	if localPort < 0 || localPort > 65535 {
		return nil, fmt.Errorf("invalid local port")
	}
	if hostSuffix == "" {
		return nil, fmt.Errorf("invalid TCP hostname suffix")
	}

	var tunnelName, hostname string
	for {
		tunnelName = name + "-" + utils.RandomCode(8)
		hostname = tunnelName + "." + hostSuffix

		s.mux.RLock()
		_, exists := s.tunnelsByHostname[hostname]
		s.mux.RUnlock()

		if !exists {
			break
		}
	}

	tunnel := &TCPTunnel{
		Name:        tunnelName,
		Hostname:    hostname,
		WebSocket:   ws,
		Connections: make(map[uint64]*TCPConnection),
		LastPong:    time.Now(),
		writeChan:   make(chan tcpWSMessage, tcpWriteQueueSize),
		done:        make(chan struct{}),
	}

	s.mux.Lock()
	s.tunnelsByHostname[hostname] = tunnel
	s.mux.Unlock()

	return tunnel, nil
}

func (s *TCPTunnelService) readLoop(tunnel *TCPTunnel) {
	for {
		messageType, payload, err := tunnel.WebSocket.ReadMessage()
		if err != nil {
			return
		}

		switch messageType {
		case websocket.TextMessage:
			s.handleControlMessage(tunnel, payload)
		case websocket.BinaryMessage:
			s.handleBinaryMessage(tunnel, payload)
		}
	}
}

func (s *TCPTunnelService) handleControlMessage(tunnel *TCPTunnel, payload []byte) {
	var msg tcpControlMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "pong":
		tunnel.mu.Lock()
		tunnel.LastPong = time.Now()
		tunnel.mu.Unlock()
	case "ping":
		tunnel.sendControl(tcpControlMessage{Type: "pong"})
	case "close":
		s.removeTCPConnection(tunnel, msg.ConnectionID, true)
	}
}

func (s *TCPTunnelService) handleBinaryMessage(tunnel *TCPTunnel, payload []byte) {
	if len(payload) < 8 || len(payload)-8 > tcpMaxFramePayload {
		return
	}

	connectionID := binary.BigEndian.Uint64(payload[:8])

	tunnel.mu.Lock()
	connection := tunnel.Connections[connectionID]
	tunnel.mu.Unlock()
	if connection == nil {
		return
	}

	if err := writeAll(connection.Conn, payload[8:]); err != nil {
		if s.removeTCPConnection(tunnel, connectionID, true) {
			tunnel.sendControl(tcpControlMessage{Type: "close", ConnectionID: connectionID})
		}
	}
}

func (s *TCPTunnelService) pipeTCPToWebSocket(tunnel *TCPTunnel, connectionID uint64, conn net.Conn) {
	buffer := make([]byte, tcpReadBufferSize)

	for {
		n, err := conn.Read(buffer)
		if n > 0 {
			if !tunnel.sendBinaryFrame(connectionID, buffer[:n]) {
				break
			}
		}
		if err != nil {
			break
		}
	}

	if s.removeTCPConnection(tunnel, connectionID, true) {
		tunnel.sendControl(tcpControlMessage{Type: "close", ConnectionID: connectionID})
	}
}

func (s *TCPTunnelService) heartbeatLoop(tunnel *TCPTunnel) {
	ticker := time.NewTicker(tcpHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tunnel.mu.Lock()
			lastPong := tunnel.LastPong
			tunnel.mu.Unlock()

			if time.Since(lastPong) > tcpHeartbeatTimeout {
				s.unregisterTunnel(tunnel)
				return
			}
			tunnel.sendControl(tcpControlMessage{Type: "ping"})
		case <-tunnel.done:
			return
		}
	}
}

func (s *TCPTunnelService) unregisterTunnel(tunnel *TCPTunnel) {
	s.mux.Lock()
	if current := s.tunnelsByHostname[tunnel.Hostname]; current == tunnel {
		delete(s.tunnelsByHostname, tunnel.Hostname)
	}
	s.mux.Unlock()

	tunnel.close()
	logger.Log("INFO", "TCP tunnel closed", []logger.LogDetail{{Key: "hostname", Value: tunnel.Hostname}})
}

func (s *TCPTunnelService) removeTCPConnection(tunnel *TCPTunnel, connectionID uint64, closeConn bool) bool {
	tunnel.mu.Lock()
	connection, exists := tunnel.Connections[connectionID]
	if exists {
		delete(tunnel.Connections, connectionID)
	}
	tunnel.mu.Unlock()

	if exists && closeConn {
		connection.Conn.Close()
	}

	return exists
}

func (t *TCPTunnel) addConnection(conn net.Conn) (uint64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.Connections) >= tcpMaxConnections {
		return 0, fmt.Errorf("connection limit reached")
	}

	t.nextID++
	connectionID := t.nextID
	t.Connections[connectionID] = &TCPConnection{
		ID:      connectionID,
		Conn:    conn,
		Created: time.Now(),
	}

	return connectionID, nil
}

func (t *TCPTunnel) writeLoop() {
	for msg := range t.writeChan {
		if err := t.WebSocket.WriteMessage(msg.messageType, msg.payload); err != nil {
			if msg.bufferedBytes > 0 {
				t.releaseConnectionBuffer(msg.connectionID, msg.bufferedBytes)
			}
			t.close()
			return
		}
		if msg.bufferedBytes > 0 {
			t.releaseConnectionBuffer(msg.connectionID, msg.bufferedBytes)
		}
	}
}

func (t *TCPTunnel) sendControl(msg tcpControlMessage) bool {
	payload, err := json.Marshal(msg)
	if err != nil {
		return false
	}
	return t.send(tcpWSMessage{messageType: websocket.TextMessage, payload: payload})
}

func (t *TCPTunnel) sendBinaryFrame(connectionID uint64, payload []byte) bool {
	if len(payload) > tcpMaxFramePayload {
		return false
	}
	if !t.reserveConnectionBuffer(connectionID, len(payload)) {
		return false
	}

	frame := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint64(frame[:8], connectionID)
	copy(frame[8:], payload)

	if !t.send(tcpWSMessage{
		messageType:   websocket.BinaryMessage,
		connectionID:  connectionID,
		bufferedBytes: len(payload),
		payload:       frame,
	}) {
		t.releaseConnectionBuffer(connectionID, len(payload))
		return false
	}

	return true
}

func (t *TCPTunnel) send(msg tcpWSMessage) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	select {
	case <-t.done:
		return false
	case t.writeChan <- msg:
		return true
	case <-time.After(10 * time.Second):
		return false
	}
}

func (t *TCPTunnel) close() {
	t.closeOnce.Do(func() {
		close(t.done)

		t.mu.Lock()
		for id, connection := range t.Connections {
			connection.Conn.Close()
			delete(t.Connections, id)
		}
		t.mu.Unlock()

		t.WebSocket.Close()
		close(t.writeChan)
	})
}

func (t *TCPTunnel) reserveConnectionBuffer(connectionID uint64, size int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	connection := t.Connections[connectionID]
	if connection == nil {
		return false
	}
	if connection.BufferedBytes+size > tcpMaxConnBuffer {
		return false
	}
	connection.BufferedBytes += size
	return true
}

func (t *TCPTunnel) releaseConnectionBuffer(connectionID uint64, size int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	connection := t.Connections[connectionID]
	if connection == nil {
		return
	}
	connection.BufferedBytes -= size
	if connection.BufferedBytes < 0 {
		connection.BufferedBytes = 0
	}
}

func tcpHostnameSuffix(host string) string {
	host = normalizeTCPHostname(host)
	if host == "" {
		return ""
	}

	if strings.HasPrefix(host, "api.") {
		host = strings.TrimPrefix(host, "api.")
	}
	if strings.HasPrefix(host, "tcp.") {
		return host
	}
	if strings.HasPrefix(host, "localhost") || net.ParseIP(host) != nil {
		return "tcp." + host
	}

	parts := strings.Split(host, ".")
	if len(parts) > 2 {
		host = strings.Join(parts[len(parts)-2:], ".")
	}

	return "tcp." + host
}

func normalizeTCPHostname(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(host, ".")
	return host
}

func writeAll(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrUnexpectedEOF
		}
		payload = payload[n:]
	}
	return nil
}
