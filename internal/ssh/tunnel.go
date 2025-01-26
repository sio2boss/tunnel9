package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"tunnel9/internal/config"

	"github.com/kevinburke/ssh_config"
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
		// If tunnel is invalid, try to log without the name prefix
		msg := fmt.Sprintf(format, args...)
		if t != nil && t.LogChan != nil {
			t.LogChan <- fmt.Sprintf("%s DEBUG %s", time.Now().Format("15:04:05"), msg)
		}
		return
	}

	msg := fmt.Sprintf("[%s] DEBUG %s", t.Config.Name, fmt.Sprintf(format, args...))
	if t.LogChan != nil {
		t.LogChan <- fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), msg)
	}
}

func (t *Tunnel) errorf(format string, args ...interface{}) {
	if t == nil || t.Client == nil {
		return
	}

	msg := fmt.Sprintf("[%s] ERROR %s", t.Config.Name, fmt.Sprintf(format, args...))
	if t.LogChan != nil {
		t.LogChan <- fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), msg)
	}
	t.updateStatus("error", fmt.Sprintf(format, args...))
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

func (t *Tunnel) measureLatency() {
	for {
		if t.Client == nil {
			time.Sleep(time.Second)
			continue
		}

		start := time.Now()
		_, err := t.Client.NewSession()
		if err != nil {
			t.Metrics.mu.Lock()
			t.Metrics.Latency = -1
			t.Metrics.mu.Unlock()
		} else {
			t.Metrics.mu.Lock()
			t.Metrics.Latency = time.Since(start)
			t.Metrics.mu.Unlock()
		}

		time.Sleep(time.Second)
	}
}

func (t *Tunnel) connect(sshconfig *ssh.ClientConfig) {
	t.logf("Starting tunnel for %s:%d", t.Config.RemoteHost, t.Config.RemotePort)
	t.updateStatus("connecting", "initializing")

	// Start latency measurement
	go t.measureLatency()

	// Start metrics updater
	ticker := time.NewTicker(time.Second)
	go func() {
		for range ticker.C {
			t.updateMetrics()
		}
	}()

	// Handle (re)connections in the background
	for {
		conn, err := t.Listener.Accept()
		if err != nil {
			t.logf("Listener closed: %v", err)
			ticker.Stop()
			t.updateStatus("stopped", "listener closed")
			return
		}
		go t.forward(conn, sshconfig)
	}
}

func (t *Tunnel) forward(localConnection net.Conn, sshconfig *ssh.ClientConfig) {
	defer localConnection.Close()

	// Parse host and port
	sshHost := t.Config.RemoteHost
	if t.Config.Bastion.Host != "" {
		sshHost = t.Config.Bastion.Host
	}
	port := "22"
	if t.Config.Bastion.Host != "" && t.Config.Bastion.Port != "" {
		port = t.Config.Bastion.Port
	}

	t.logf(fmt.Sprintf("connecting to SSH server: %s", net.JoinHostPort(sshHost, port)))
	t.updateStatus("connecting", "connecting to SSH server")
	dialer := net.Dialer{
		Timeout: 10 * time.Second,
	}
	conn, err := dialer.Dial("tcp", net.JoinHostPort(sshHost, port))
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			t.errorf("connection timed out")
		} else {
			t.errorf("network error: %s", err)
		}
		return
	}

	// Create SSH connection
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, net.JoinHostPort(sshHost, port), sshconfig)
	if err != nil {
		t.errorf("SSH connection failed: %s", err)
		return
	}

	serverConn := ssh.NewClient(sshConn, chans, reqs)
	t.Client = serverConn
	defer serverConn.Close()

	remoteHost := "localhost"
	if t.Config.Bastion.Host != "" {
		remoteHost = t.Config.RemoteHost
	}

	actualRemoteAddress := net.JoinHostPort(remoteHost, fmt.Sprintf("%d", t.Config.RemotePort))
	t.logf(fmt.Sprintf("establishing remote connection: %s", actualRemoteAddress))
	t.updateStatus("connecting", "establishing remote connection")
	remoteConnection, err := serverConn.Dial("tcp", actualRemoteAddress)
	if err != nil {
		t.errorf("connection failed")
		return
	}
	defer remoteConnection.Close()

	t.updateStatus("active", "tunnel established")

	// Copy bidirectionally with metrics
	copyConn := func(writer, reader net.Conn, isUpload bool) {
		direction := "download"
		if isUpload {
			direction = "upload"
		}

		buf := make([]byte, 32*1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				_, werr := writer.Write(buf[:n])
				if werr != nil {
					t.errorf("Writing %s data: %v", direction, werr)
					break
				}

				t.Metrics.mu.Lock()
				if isUpload {
					t.Metrics.BytesOut += int64(n)
				} else {
					t.Metrics.BytesIn += int64(n)
				}
				t.Metrics.mu.Unlock()
			}
			if err != nil {
				if err != io.EOF {
					t.errorf("Reading %s data: %v", direction, err)
				}
				break
			}
		}
	}

	// Start both copy operations and wait for them to complete
	done := make(chan bool, 2)
	go func() {
		copyConn(remoteConnection, localConnection, true) // Upload
		done <- true
	}()
	go func() {
		copyConn(localConnection, remoteConnection, false) // Download
		done <- true
	}()

	// Wait for both copies to finish
	<-done
	<-done

	t.logf("connection closed, waiting for new connection")
	t.updateStatus("connecting", "connection closed, waiting for new connection")
}

func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

func (t *Tunnel) getSSHConfig() (*ssh.ClientConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Try ECDSA first, then RSA
	var key []byte
	keyPaths := []string{
		filepath.Join(home, ".ssh", "id_ecdsa"),
		filepath.Join(home, ".ssh", "id_rsa"),
	}

	var lastErr error
	var auths []ssh.AuthMethod
	for _, keyPath := range keyPaths {
		key, err = os.ReadFile(keyPath)
		if err != nil {
			t.logf("Failed to find key at %s", keyPath)
			continue
		}
		lastErr = err

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}

		// Add signer to config
		auths = append(auths, ssh.PublicKeys(signer))
		t.logf("Using SSH key: %s", keyPath)
	}

	if len(auths) == 0 {
		return nil, fmt.Errorf("no SSH keys found: %v", lastErr)
	}

	// Load SSH config file
	configFile, err := os.Open(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		t.logf("Failed to open SSH config: %v", err)
	} else {
		defer configFile.Close()
		sshConfig, err := ssh_config.Decode(configFile)
		if err != nil {
			t.logf("Failed to parse SSH config: %v", err)
		} else {

			if host, _ := sshConfig.Get(t.Config.RemoteHost, "HostName"); host != "" {
				t.logf("Replacing RemoteHost: %s from ssh_config with: %s", t.Config.RemoteHost, host)
				t.Config.RemoteHost = host
			}

			if t.Config.Bastion.Host != "" {
				bastionHost := t.Config.Bastion.Host
				if host, _ := sshConfig.Get(bastionHost, "HostName"); host != "" {
					t.logf("Replacing BastionHost: %s from ssh_config with: %s", bastionHost, host)
					t.Config.Bastion.Host = host
				}
				if port, _ := sshConfig.Get(bastionHost, "Port"); port != "" {
					t.logf("Replacing BastionPort: %s from ssh_config with: %s", t.Config.Bastion.Port, port)
					t.Config.Bastion.Port = port
				}
				if user, _ := sshConfig.Get(bastionHost, "User"); user != "" {
					t.logf("Replacing BastionUser: %s from ssh_config with: %s", t.Config.Bastion.User, user)
					t.Config.Bastion.User = user
				}
				t.logf("BastionHost: %s", bastionHost)
				if identityFile, _ := sshConfig.Get(bastionHost, "IdentityFile"); identityFile != "" {
					t.logf("adding identity file: %s", identityFile)
					key, err = os.ReadFile(identityFile)
					if err != nil {
						t.logf("Failed to find key at %s", identityFile)
					} else {
						signer, err := ssh.ParsePrivateKey(key)
						if err != nil {
							t.errorf("failed to parse private key: %w", err)
						} else {
							auths = []ssh.AuthMethod{ssh.PublicKeys(signer)}
							t.logf("Using SSH key: %s", identityFile)
						}

					}
				}
			}

		}

	}

	// If using bastion, override the user with bastion user
	user := t.Config.User
	if t.Config.Bastion.Host != "" && t.Config.Bastion.User != "" {
		user = t.Config.Bastion.User
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Implement proper host key verification
		Timeout:         10 * time.Second,
	}

	return config, nil
}
