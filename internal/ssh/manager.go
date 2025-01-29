package ssh

import (
	"fmt"
	"net"
	"time"
	"tunnel9/internal/config"
)

type TunnelManager struct {
	tunnels    map[string]*Tunnel
	LogChan    chan string
	StatusChan chan TunnelStatus
}

func NewTunnelManager() *TunnelManager {
	return &TunnelManager{
		tunnels:    make(map[string]*Tunnel),
		LogChan:    make(chan string, 100),     // Buffered channel to prevent blocking
		StatusChan: make(chan TunnelStatus, 5), // Small buffer for status updates
	}
}

func formatBytes(bytes float64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%.1f B/s", bytes)
	}
	div, exp := float64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB/s", bytes/div, "KMGTPE"[exp])
}

func formatLatency(d time.Duration) string {
	if d < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}

func (tm *TunnelManager) GetMetrics(id string) string {
	tunnel, exists := tm.tunnels[id]
	if !exists {
		return "--"
	}

	tunnel.Metrics.mu.Lock()
	defer tunnel.Metrics.mu.Unlock()

	return fmt.Sprintf("↑%s ↓%s [%s]",
		formatBytes(tunnel.Metrics.CurrentRateOut),
		formatBytes(tunnel.Metrics.CurrentRateIn),
		formatLatency(tunnel.Metrics.Latency))
}

func (tm *TunnelManager) CreateTunnel(id string, config config.TunnelConfig) *Tunnel {
	// Check if tunnel already exists
	if _, exists := tm.tunnels[id]; exists {
		return tm.tunnels[id]
	}

	// Create tunnel with log channel
	tunnel := &Tunnel{
		ID:         id,
		Client:     nil,
		Config:     config,
		LogChan:    make(chan string, 50),      // Buffered channel for tunnel-specific logs
		StatusChan: make(chan TunnelStatus, 2), // Small buffer for status updates
	}

	// Start goroutine to forward tunnel status to manager's status channel
	go func() {
		for status := range tunnel.StatusChan {
			tm.StatusChan <- status
		}
	}()

	// Store the tunnel
	tm.tunnels[id] = tunnel
	return tunnel
}

func (tm *TunnelManager) StartTunnel(tunnel *Tunnel) error {
	// Check if port is available
	if !isPortAvailable(tunnel.Config.BindAddress, tunnel.Config.LocalPort) {
		tunnel.errorf("port already in use")
		return fmt.Errorf("port already in use")
	}

	// Get SSH config
	sshconfig, err := tunnel.getSSHConfig()
	if err != nil {
		tunnel.errorf("failed to get SSH config")
		return fmt.Errorf("failed to get SSH config")
	}

	// Start local listener
	bindAddr := tunnel.Config.BindAddress
	if bindAddr == "" {
		bindAddr = "0.0.0.0"
	}
	tunnel.Listener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", bindAddr, tunnel.Config.LocalPort))
	if err != nil {
		tunnel.errorf("failed to listen on port %d", tunnel.Config.LocalPort)
		return fmt.Errorf("failed to listen on port %d", tunnel.Config.LocalPort)
	}

	// Start goroutine to forward tunnel logs to manager's log channel
	go func() {
		for msg := range tunnel.LogChan {
			tm.LogChan <- msg
		}
	}()

	// Start the tunnel
	go tunnel.connect(sshconfig)

	return nil
}

func (tm *TunnelManager) StopTunnel(id string) error {
	tunnel, exists := tm.tunnels[id]
	if !exists {
		return nil
	}

	tunnel.logf("Tunnel connection closed for %s", tunnel.Config.Name)
	if tunnel.Listener != nil {
		tunnel.Listener.Close()
		tunnel.Listener = nil
	}
	if tunnel.Client != nil {
		tunnel.Client.Close()
		tunnel.Client = nil
	}

	time.Sleep(time.Second / 2)

	// Close the tunnel's channels
	close(tunnel.LogChan)
	tunnel.LogChan = nil
	close(tunnel.StatusChan)
	tunnel.StatusChan = nil
	delete(tm.tunnels, id)
	return nil
}
