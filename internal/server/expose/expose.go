package expose

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pedroborgesdev/tunnerse-cli/internal/server/config"
	"github.com/pedroborgesdev/tunnerse-cli/internal/server/logger"
)

var (
	routes       = make(map[string]string)
	redirectList = make([]string, 0)
	tlsCertFile  string
	tlsKeyFile   string
)

type TCPConnHandler interface {
	IsTCPTunnelHostname(hostname string) bool
	HandleExternalConnection(hostname string, conn net.Conn)
}

type channelListener struct {
	conns chan net.Conn
	addr  net.Addr
	done  chan struct{}
}

func (l *channelListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		if conn == nil {
			return nil, net.ErrClosed
		}
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *channelListener) Close() error {
	select {
	case <-l.done:
	default:
		close(l.done)
	}
	return nil
}

func (l *channelListener) Addr() net.Addr {
	return l.addr
}

func loadConfig(path string) error {
	routes = make(map[string]string)
	redirectList = make([]string, 0)
	tlsCertFile = ""
	tlsKeyFile = ""

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var section string
	domainsFound := false
	domainsCount := 0
	redirectsCount := 0
	tlsFound := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(line[1 : len(line)-1])
			if section == "domains" {
				domainsFound = true
			}
			if section == "tls" {
				tlsFound = true
			}
			continue
		}

		switch section {
		case "domains":
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid line on config: %s", line)
			}
			domain := strings.TrimSpace(parts[0])
			port := strings.TrimSpace(parts[1])

			if domain == "" {
				return fmt.Errorf("invalid or null domain")
			}
			if port == "" {
				return fmt.Errorf("invalid or null port")
			}

			routes[domain] = port
			domainsCount++

		case "redirects":
			redirectList = append(redirectList, strings.ToLower(line))
			redirectsCount++

		case "tls":
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid line on config: %s", line)
			}
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			value := strings.TrimSpace(parts[1])
			if value == "" {
				return fmt.Errorf("invalid or null tls value for %s", key)
			}

			switch key {
			case "cert_file":
				tlsCertFile = resolveConfigPath(path, value)
			case "key_file":
				tlsKeyFile = resolveConfigPath(path, value)
			default:
				return fmt.Errorf("unknown tls config key: %s", key)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if !domainsFound {
		return fmt.Errorf("config file must contain [domains] section")
	}
	if domainsCount == 0 {
		return fmt.Errorf("[domains] section is empty, configure at least one domain")
	}
	if !tlsFound {
		return fmt.Errorf("config file must contain [tls] section")
	}
	if tlsCertFile == "" {
		return fmt.Errorf("[tls] cert_file is required")
	}
	if tlsKeyFile == "" {
		return fmt.Errorf("[tls] key_file is required")
	}

	return nil
}

func resolveConfigPath(configPath, value string) string {
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(filepath.Dir(configPath), value)
}

func newReverseProxy(target string) *httputil.ReverseProxy {
	u, _ := url.Parse(target)
	return httputil.NewSingleHostReverseProxy(u)
}

func handler(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(strings.Split(r.Host, ":")[0])

	for domain, port := range routes {
		domain = strings.ToLower(domain)

		if strings.HasPrefix(domain, "*.") {
			base := strings.TrimPrefix(domain, "*.")
			if strings.HasSuffix(host, "."+base) || host == base {
				target := fmt.Sprintf("http://localhost:%s", port)
				newReverseProxy(target).ServeHTTP(w, r)
				return
			}
		} else if host == domain {
			target := fmt.Sprintf("http://localhost:%s", port)
			newReverseProxy(target).ServeHTTP(w, r)
			return
		}
	}

	http.Error(w, "domain not configured", http.StatusNotFound)
}

func StartExpose(tcpHandler TCPConnHandler) (<-chan error, error) {
	if err := loadConfig("tunnerse.config"); err != nil {
		return nil, fmt.Errorf("error to load config: %v", err)
	}

	errCh := make(chan error, 2)

	redirectSrv := &http.Server{
		Addr:              ":80",
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			url := "https://" + r.Host + r.URL.String()
			http.Redirect(w, r, url, http.StatusMovedPermanently)
		}),
	}

	writeTimeout := time.Duration(config.AppConfig.TUNNEL_REQUEST_TIMEOUT+30) * time.Second
	if writeTimeout < 90*time.Second {
		writeTimeout = 90 * time.Second
	}

	cert, err := tls.LoadX509KeyPair(tlsCertFile, tlsKeyFile)
	if err != nil {
		return nil, fmt.Errorf("error to load TLS certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	rawTLSListener, err := net.Listen("tcp", ":443")
	if err != nil {
		return nil, fmt.Errorf("https listener error: %w", err)
	}

	httpTLSListener := &channelListener{
		conns: make(chan net.Conn, 1024),
		addr:  rawTLSListener.Addr(),
		done:  make(chan struct{}),
	}

	httpsSrv := &http.Server{
		Addr:              ":443",
		Handler:           http.HandlerFunc(handler),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       90 * time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       120 * time.Second,
	}

	logger.Log("INFO", "Expose iniciado", []logger.LogDetail{
		{Key: "redirect", Value: ":80"},
		{Key: "https", Value: ":443"},
		{Key: "certFile", Value: tlsCertFile},
		{Key: "keyFile", Value: tlsKeyFile},
	})

	go func() {
		if err := redirectSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("redirect server error: %w", err)
		}
	}()

	go func() {
		if err := httpsSrv.Serve(httpTLSListener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("https server error: %w", err)
		}
	}()

	go routeTLSConnections(rawTLSListener, tlsConfig, httpTLSListener, tcpHandler, errCh)

	return errCh, nil
}

func Expose() error {
	errCh, err := StartExpose(nil)
	if err != nil {
		return err
	}
	return <-errCh
}

func routeTLSConnections(listener net.Listener, tlsConfig *tls.Config, httpTLSListener *channelListener, tcpHandler TCPConnHandler, errCh chan<- error) {
	for {
		rawConn, err := listener.Accept()
		if err != nil {
			errCh <- fmt.Errorf("tls accept error: %w", err)
			return
		}

		go routeTLSConnection(rawConn, tlsConfig, httpTLSListener, tcpHandler)
	}
}

func routeTLSConnection(rawConn net.Conn, tlsConfig *tls.Config, httpTLSListener *channelListener, tcpHandler TCPConnHandler) {
	tlsConn := tls.Server(rawConn, tlsConfig)
	if err := tlsConn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		rawConn.Close()
		return
	}
	if err := tlsConn.Handshake(); err != nil {
		rawConn.Close()
		return
	}
	_ = tlsConn.SetDeadline(time.Time{})

	hostname := strings.ToLower(strings.TrimSuffix(tlsConn.ConnectionState().ServerName, "."))
	if tcpHandler != nil && tcpHandler.IsTCPTunnelHostname(hostname) {
		tcpHandler.HandleExternalConnection(hostname, tlsConn)
		return
	}

	select {
	case httpTLSListener.conns <- tlsConn:
	case <-httpTLSListener.done:
		tlsConn.Close()
	}
}
