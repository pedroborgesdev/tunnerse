package jobs

import (
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pedroborgesdev/tunnerse-cli/internal/daemon/logger"
)

const (
	tcpFramePayloadMax = 64 << 10
	tcpConnBufferMax   = 1 << 20
	tcpReadBufferSize  = 32 << 10
	tcpWriteQueueSize  = 256
)

type TCPTunnelJob struct {
	Name      string
	LocalPort string
	WSURL     string
	Hostname  string
	TunnelID  string

	ws          *websocket.Conn
	connections map[uint64]*tcpLocalConnection
	writeChan   chan tcpJobWSMessage
	done        chan struct{}
	closeOnce   sync.Once
	mu          sync.Mutex
}

type tcpLocalConnection struct {
	conn          net.Conn
	bufferedBytes int
}

type tcpJobWSMessage struct {
	messageType   int
	connectionID  uint64
	bufferedBytes int
	payload       []byte
}

type tcpJobControlMessage struct {
	Type         string `json:"type"`
	TunnelName   string `json:"tunnel_name,omitempty"`
	LocalPort    int    `json:"local_port,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
	ConnectionID uint64 `json:"connection_id,omitempty"`
}

func NewTCPTunnelJob(name, localPort, wsURL string) *TCPTunnelJob {
	return &TCPTunnelJob{
		Name:        name,
		LocalPort:   localPort,
		WSURL:       wsURL,
		connections: make(map[uint64]*tcpLocalConnection),
		writeChan:   make(chan tcpJobWSMessage, tcpWriteQueueSize),
		done:        make(chan struct{}),
	}
}

func (j *TCPTunnelJob) Start() (string, error) {
	localPort, err := parseTCPPort(j.LocalPort)
	if err != nil {
		return "", err
	}

	conn, _, err := tcpWebSocketDialer().Dial(j.WSURL, nil)
	if err != nil {
		return "", fmt.Errorf("connect TCP websocket: %w", err)
	}
	j.ws = conn

	go j.writeLoop()
	if !j.sendControl(tcpJobControlMessage{
		Type:       "register",
		TunnelName: j.Name,
		LocalPort:  localPort,
	}) {
		j.Stop()
		return "", fmt.Errorf("send TCP register")
	}

	messageType, payload, err := j.ws.ReadMessage()
	if err != nil {
		j.Stop()
		return "", fmt.Errorf("read TCP register response: %w", err)
	}
	if messageType != websocket.TextMessage {
		j.Stop()
		return "", fmt.Errorf("unexpected TCP register response")
	}

	var registered tcpJobControlMessage
	if err := json.Unmarshal(payload, &registered); err != nil {
		j.Stop()
		return "", fmt.Errorf("decode TCP register response: %w", err)
	}
	if registered.Type != "registered" || registered.Hostname == "" {
		j.Stop()
		return "", fmt.Errorf("TCP tunnel registration rejected")
	}

	j.Hostname = registered.Hostname
	j.TunnelID = tunnelNameFromHostname(registered.Hostname)

	go j.readLoop()
	return registered.Hostname, nil
}

func (j *TCPTunnelJob) Done() <-chan struct{} {
	return j.done
}

func (j *TCPTunnelJob) Stop() {
	j.closeOnce.Do(func() {
		close(j.done)

		j.mu.Lock()
		for id, connection := range j.connections {
			connection.conn.Close()
			delete(j.connections, id)
		}
		j.mu.Unlock()

		if j.ws != nil {
			j.ws.Close()
		}
		close(j.writeChan)
	})
}

func (j *TCPTunnelJob) readLoop() {
	defer j.Stop()

	for {
		messageType, payload, err := j.ws.ReadMessage()
		if err != nil {
			return
		}

		switch messageType {
		case websocket.TextMessage:
			j.handleControlMessage(payload)
		case websocket.BinaryMessage:
			j.handleBinaryMessage(payload)
		}
	}
}

func (j *TCPTunnelJob) handleControlMessage(payload []byte) {
	var msg tcpJobControlMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "new_connection":
		j.openLocalConnection(msg.ConnectionID)
	case "close":
		j.removeConnection(msg.ConnectionID, true)
	case "ping":
		j.sendControl(tcpJobControlMessage{Type: "pong"})
	case "pong":
	}
}

func (j *TCPTunnelJob) handleBinaryMessage(payload []byte) {
	if len(payload) < 8 || len(payload)-8 > tcpFramePayloadMax {
		return
	}

	connectionID := binary.BigEndian.Uint64(payload[:8])
	j.mu.Lock()
	connection := j.connections[connectionID]
	j.mu.Unlock()
	if connection == nil {
		return
	}

	if err := tcpWriteAll(connection.conn, payload[8:]); err != nil {
		if j.removeConnection(connectionID, true) {
			j.sendControl(tcpJobControlMessage{Type: "close", ConnectionID: connectionID})
		}
	}
}

func (j *TCPTunnelJob) openLocalConnection(connectionID uint64) {
	address := "127.0.0.1:" + j.LocalPort
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		j.sendControl(tcpJobControlMessage{Type: "close", ConnectionID: connectionID})
		logger.Log("ERROR", "Failed to connect to local TCP service", []logger.LogDetail{
			{Key: "connection_id", Value: connectionID},
			{Key: "address", Value: address},
			{Key: "error", Value: err.Error()},
		})
		return
	}

	j.mu.Lock()
	j.connections[connectionID] = &tcpLocalConnection{conn: conn}
	j.mu.Unlock()

	go j.pipeLocalToWebSocket(connectionID, conn)
}

func (j *TCPTunnelJob) pipeLocalToWebSocket(connectionID uint64, conn net.Conn) {
	buffer := make([]byte, tcpReadBufferSize)

	for {
		n, err := conn.Read(buffer)
		if n > 0 {
			if !j.sendBinaryFrame(connectionID, buffer[:n]) {
				break
			}
		}
		if err != nil {
			break
		}
	}

	if j.removeConnection(connectionID, true) {
		j.sendControl(tcpJobControlMessage{Type: "close", ConnectionID: connectionID})
	}
}

func (j *TCPTunnelJob) writeLoop() {
	for msg := range j.writeChan {
		if err := j.ws.WriteMessage(msg.messageType, msg.payload); err != nil {
			if msg.bufferedBytes > 0 {
				j.releaseConnectionBuffer(msg.connectionID, msg.bufferedBytes)
			}
			j.Stop()
			return
		}
		if msg.bufferedBytes > 0 {
			j.releaseConnectionBuffer(msg.connectionID, msg.bufferedBytes)
		}
	}
}

func (j *TCPTunnelJob) sendControl(msg tcpJobControlMessage) bool {
	payload, err := json.Marshal(msg)
	if err != nil {
		return false
	}
	return j.send(tcpJobWSMessage{messageType: websocket.TextMessage, payload: payload})
}

func (j *TCPTunnelJob) sendBinaryFrame(connectionID uint64, payload []byte) bool {
	if len(payload) > tcpFramePayloadMax {
		return false
	}
	if !j.reserveConnectionBuffer(connectionID, len(payload)) {
		return false
	}

	frame := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint64(frame[:8], connectionID)
	copy(frame[8:], payload)

	if !j.send(tcpJobWSMessage{
		messageType:   websocket.BinaryMessage,
		connectionID:  connectionID,
		bufferedBytes: len(payload),
		payload:       frame,
	}) {
		j.releaseConnectionBuffer(connectionID, len(payload))
		return false
	}

	return true
}

func (j *TCPTunnelJob) send(msg tcpJobWSMessage) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	select {
	case <-j.done:
		return false
	case j.writeChan <- msg:
		return true
	case <-time.After(10 * time.Second):
		return false
	}
}

func (j *TCPTunnelJob) removeConnection(connectionID uint64, closeConn bool) bool {
	j.mu.Lock()
	connection, exists := j.connections[connectionID]
	if exists {
		delete(j.connections, connectionID)
	}
	j.mu.Unlock()

	if exists && closeConn {
		connection.conn.Close()
	}

	return exists
}

func (j *TCPTunnelJob) reserveConnectionBuffer(connectionID uint64, size int) bool {
	j.mu.Lock()
	defer j.mu.Unlock()

	connection := j.connections[connectionID]
	if connection == nil {
		return false
	}
	if connection.bufferedBytes+size > tcpConnBufferMax {
		return false
	}
	connection.bufferedBytes += size
	return true
}

func (j *TCPTunnelJob) releaseConnectionBuffer(connectionID uint64, size int) {
	j.mu.Lock()
	defer j.mu.Unlock()

	connection := j.connections[connectionID]
	if connection == nil {
		return
	}
	connection.bufferedBytes -= size
	if connection.bufferedBytes < 0 {
		connection.bufferedBytes = 0
	}
}

func tcpWebSocketDialer() *websocket.Dialer {
	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &dialer
}

func tcpWriteAll(w io.Writer, payload []byte) error {
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

func parseTCPPort(value string) (int, error) {
	port, err := net.LookupPort("tcp", value)
	if err != nil {
		return 0, err
	}
	if port < 0 || port > 65535 {
		return 0, fmt.Errorf("invalid port")
	}
	return port, nil
}

func tunnelNameFromHostname(hostname string) string {
	host, _, err := net.SplitHostPort(hostname)
	if err == nil {
		hostname = host
	}
	parts := strings.Split(hostname, ".")
	if len(parts) == 0 {
		return hostname
	}
	return parts[0]
}
