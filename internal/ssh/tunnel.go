package ssh

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"tunnel9/internal/config"

	"golang.org/x/crypto/ssh"
)

type TunnelMetrics struct {
	BytesIn        int64
	BytesOut       int64
	LastBytesIn    int64
	LastBytesOut   int64
	LastUpdate     time.Time
	CurrentRateIn  float64 // bytes per second
	CurrentRateOut float64 // bytes per second
	Latency        time.Duration
	mu             sync.Mutex
}

type TunnelStatus struct {
	ID      string
	State   string // "stopped", "active", "error"
	Message string
}

type TunnelOptions struct {
	Host       string
	LocalPort  int
	RemotePort int
	RemoteHost string
}

type Tunnel struct {
	ID         string
	Client     *ssh.Client
	Config     config.TunnelConfig
	LogChan    chan string
	StatusChan chan TunnelStatus
	Listener   net.Listener
	Metrics    TunnelMetrics
	stopChan   chan struct{} // Add stop channel for clean shutdown
	clientMu   sync.RWMutex  // Protect SSH client access
}

func (t *Tunnel) updateStatus(state string, message string) {
	if t != nil && t.StatusChan != nil {
		t.StatusChan <- TunnelStatus{
			ID:      t.ID,
			State:   state,
			Message: message,
		}
	}
}

func (t *Tunnel) logf(format string, args ...interface{}) {
	if t == nil || t.Config.Name == "" {
		return
	}

	msg := fmt.Sprintf("[%s] DEBUG %s", t.Config.Name, fmt.Sprintf(format, args...))
	if t.LogChan != nil {
		t.LogChan <- fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), msg)
	}
}

func (t *Tunnel) errorf(format string, args ...interface{}) {
	if t == nil {
		return
	}

	msg := fmt.Sprintf("[%s] ERROR %s", t.Config.Name, fmt.Sprintf(format, args...))
	if t.LogChan != nil {
		t.LogChan <- fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), msg)
	}
	t.updateStatus("error", "failed, see logs")
}

func (t *Tunnel) updateMetrics() {
	t.Metrics.mu.Lock()
	defer t.Metrics.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(t.Metrics.LastUpdate).Seconds()
	if elapsed > 0 {
		bytesInDiff := t.Metrics.BytesIn - t.Metrics.LastBytesIn
		bytesOutDiff := t.Metrics.BytesOut - t.Metrics.LastBytesOut

		t.Metrics.CurrentRateIn = float64(bytesInDiff) / elapsed
		t.Metrics.CurrentRateOut = float64(bytesOutDiff) / elapsed

		t.Metrics.LastBytesIn = t.Metrics.BytesIn
		t.Metrics.LastBytesOut = t.Metrics.BytesOut
		t.Metrics.LastUpdate = now
	}
}

// isSSHClientHealthy checks if the SSH client is still responsive
func (t *Tunnel) isSSHClientHealthy() bool {
	if t == nil {
		return false
	}

	t.clientMu.RLock()
	client := t.Client
	t.clientMu.RUnlock()

	if client == nil {
		return false
	}

	// Try to create a session with a timeout
	done := make(chan bool, 1)
	var healthy bool

	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Recover from any panics in session creation
				healthy = false
			}
			done <- true
		}()

		session, err := client.NewSession()
		if err == nil {
			session.Close()
			healthy = true
		}
	}()

	// Wait for the health check with a timeout
	select {
	case <-done:
		return healthy
	case <-time.After(2 * time.Second):
		// Timeout - client is probably not healthy
		return false
	}
}

// isConnectionError checks if the error indicates a connection problem that requires SSH client recreation
func (t *Tunnel) isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	connectionErrors := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"network is unreachable",
		"no route to host",
		"timeout",
		"connection timed out",
		"ssh: disconnect",
		"ssh: connection lost",
		"use of closed network connection",
	}

	for _, connErr := range connectionErrors {
		if strings.Contains(errStr, connErr) {
			return true
		}
	}

	return false
}

func (t *Tunnel) connect(sshconfig *ssh.ClientConfig) {
	t.logf("Starting tunnel")

	// Initialize stop channel
	t.stopChan = make(chan struct{})

	// Start combined metrics and latency updater
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Log the panic but don't crash
				if t != nil && t.LogChan != nil {
					t.logf("Metrics updater panic recovered: %v", r)
				}
			}
		}()

		for {
			select {
			case <-t.stopChan:
				return
			case <-ticker.C:
				if t == nil || t.Client == nil {
					continue
				}
				// Update metrics
				t.updateMetrics()

				// Measure latency
				t.clientMu.RLock()
				client := t.Client
				t.clientMu.RUnlock()

				if client == nil {
					t.Metrics.mu.Lock()
					t.Metrics.Latency = -1
					t.Metrics.mu.Unlock()
					continue
				}

				start := time.Now()
				session, err := client.NewSession()
				t.Metrics.mu.Lock()
				if err != nil {
					t.Metrics.Latency = -1
					t.Metrics.mu.Unlock()
					t.logf("SSH client health check failed: %v", err)
					// Close the client so the next connection attempt creates a new one
					t.clientMu.Lock()
					if t.Client != nil {
						t.Client.Close()
						t.Client = nil
					}
					t.clientMu.Unlock()
					continue
				}
				t.Metrics.Latency = time.Since(start)
				session.Close()
				t.Metrics.mu.Unlock()
			}
		}
	}()

	// Handle (re)connections in the background
	t.updateStatus("connecting", "waiting for traffic")
	for {
		// Check if we should stop
		select {
		case <-t.stopChan:
			t.logf("Tunnel stopping")
			return
		default:
		}

		// Signal that this is an error condition, not a normal stop
		if t.Listener == nil {
			t.errorf("Listener cannot accept connections")
			t.updateStatus("error", "cannot accept connections")
			return
		}

		t.Listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))

		conn, err := t.Listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// This is just a timeout, continue to check stopChan
				continue
			}
			t.logf("Listener closed: %v", err)
			return
		}
		go t.forward(conn, sshconfig)
	}
}

func (t *Tunnel) Stop() {
	if t.stopChan != nil {
		close(t.stopChan)
	}

	t.clientMu.Lock()
	if t.Client != nil {
		t.Client.Close()
		t.Client = nil
	}
	t.clientMu.Unlock()
}

func figureOutRemoteVsBastion(config config.TunnelConfig) (*Endpoint, *Endpoint) {

	// Start with bastion mode
	sshHost := config.Bastion.Host
	sshPort := config.Bastion.Port
	remoteHost := config.RemoteHost
	remotePort := config.RemotePort

	// If bastion host is not set, use remote host
	if sshHost == "" {
		sshHost = config.RemoteHost
		remoteHost = "localhost"
	}

	// Default to port 22 if not set
	if sshPort == 0 {
		sshPort = 22
	}

	remoteEndpoint := NewEndpoint(remoteHost, remotePort)
	sshEndpoint := NewEndpoint(sshHost, sshPort)
	return sshEndpoint, remoteEndpoint
}

func (t *Tunnel) forward(localConnection net.Conn, sshconfig *ssh.ClientConfig) {
	defer localConnection.Close()

	// Check if tunnel is being shut down
	if t == nil {
		return
	}

	select {
	case <-t.stopChan:
		t.logf("Tunnel stopping, aborting forward")
		return
	default:
	}

	// Parse host and port
	sshEndpoint, remoteEndpoint := figureOutRemoteVsBastion(t.Config)

	// Check if SSH client is healthy and reconnect if necessary
	t.clientMu.Lock()
	needsHealthCheck := t.Client != nil
	t.clientMu.Unlock()

	if needsHealthCheck && !t.isSSHClientHealthy() {
		t.logf("SSH client appears unhealthy, closing and reconnecting")
		t.clientMu.Lock()
		if t.Client != nil {
			t.Client.Close()
			t.Client = nil
		}
		t.clientMu.Unlock()
	}

	// Only establish a new client if we don't have one or if it's closed
	var isFirstConnect bool = false
	t.clientMu.Lock()
	if t.Client == nil {
		isFirstConnect = true
		t.logf("connecting to SSH server (1/2): %s", sshEndpoint.String())
		t.updateStatus("connecting", "connecting to server")
		client, err := ssh.Dial("tcp", sshEndpoint.String(), sshconfig)
		if err != nil {
			t.errorf("SSH connection failed: %v (user: %s, address: %s)", err, sshconfig.User, sshEndpoint)
			t.updateStatus("error", fmt.Sprintf("SSH connection failed: %v", err))
			if t.Client != nil {
				t.Client.Close()
				t.Client = nil
			}
			t.clientMu.Unlock()
			return
		}
		t.Client = client
	}
	client := t.Client
	t.clientMu.Unlock()

	if isFirstConnect {
		t.logf("connecting to remote server (2/2): %s", remoteEndpoint.String())
		t.updateStatus("active", "establishing remote connection")
	}

	// Retry remote connection with exponential backoff
	maxRetries := 3
	baseDelay := time.Second

	var remoteConnection net.Conn
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Check if we should stop before each attempt
		select {
		case <-t.stopChan:
			t.logf("Tunnel stopping during connection attempt")
			return
		default:
		}

		if client == nil {
			t.errorf("SSH client became nil during connection attempt")
			return
		}

		remoteConnection, err = client.Dial("tcp", remoteEndpoint.String())
		if err == nil {
			break
		}

		t.logf("connection failed to remote target (attempt %d/%d): %v", attempt+1, maxRetries, err)

		// If this is the last attempt or SSH client seems broken, close it
		if attempt == maxRetries-1 || t.isConnectionError(err) {
			t.errorf("connection failed to remote target after %d attempts: %v", maxRetries, err)
			t.updateStatus("error", fmt.Sprintf("remote connection failed: %v", err))
			// Close and nil the client so next connection will create a fresh one
			t.clientMu.Lock()
			if t.Client != nil {
				t.Client.Close()
				t.Client = nil
			}
			t.clientMu.Unlock()
			return
		}

		// Wait before retrying with exponential backoff
		delay := time.Duration(attempt+1) * baseDelay
		t.logf("retrying remote connection in %v", delay)
		time.Sleep(delay)
	}

	defer remoteConnection.Close()

	if isFirstConnect {
		t.updateStatus("active", "tunnel established")
	}

	// Copy bidirectionally with metrics
	copyConn := func(writer, reader net.Conn, direction string) {
		buf := make([]byte, 32*1024)
		for {
			// Check if we should stop
			select {
			case <-t.stopChan:
				return
			default:
			}

			n, err := reader.Read(buf)
			if n > 0 {
				_, werr := writer.Write(buf[:n])
				if werr != nil {
					t.logf("Writing %s data: %v", direction, werr)
					break
				}

				t.Metrics.mu.Lock()
				if direction == "upload" {
					t.Metrics.BytesOut += int64(n)
				} else {
					t.Metrics.BytesIn += int64(n)
				}
				t.Metrics.mu.Unlock()
			}
			if err != nil {
				if err != io.EOF {
					t.logf("Reading %s data: %v", direction, err)
				}
				break
			}
		}
	}

	// Start both copy operations and wait for them to complete
	done := make(chan bool, 2)
	go func() {
		copyConn(remoteConnection, localConnection, "upload")
		done <- true
	}()
	go func() {
		copyConn(localConnection, remoteConnection, "download")
		done <- true
	}()

	// Wait for both copies to finish or stop signal
	finished := 0
	for finished < 2 {
		select {
		case <-done:
			finished++
		case <-t.stopChan:
			// Force close connections to unblock the copy operations
			remoteConnection.Close()
			localConnection.Close()
			return
		}
	}
}
