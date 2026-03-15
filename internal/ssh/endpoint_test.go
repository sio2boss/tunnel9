package ssh

import (
	"errors"
	"testing"
	"time"

	"tunnel9/internal/config"
)

func TestNewEndpointFromString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Endpoint
	}{
		{
			name:  "simple hostname",
			input: "example.com",
			expected: Endpoint{
				Host: "example.com",
				Port: 0,
				User: "",
			},
		},
		{
			name:  "hostname with port",
			input: "example.com:8080",
			expected: Endpoint{
				Host: "example.com",
				Port: 8080,
				User: "",
			},
		},
		{
			name:  "user and hostname",
			input: "user@example.com",
			expected: Endpoint{
				Host: "example.com",
				Port: 0,
				User: "user",
			},
		},
		{
			name:  "user, hostname and port",
			input: "user@example.com:22",
			expected: Endpoint{
				Host: "example.com",
				Port: 22,
				User: "user",
			},
		},
		{
			name:  "localhost",
			input: "localhost",
			expected: Endpoint{
				Host: "localhost",
				Port: 0,
				User: "",
			},
		},
		{
			name:  "IP address with port",
			input: "192.168.1.100:3306",
			expected: Endpoint{
				Host: "192.168.1.100",
				Port: 3306,
				User: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewEndpointFromString(tt.input)

			if result.Host != tt.expected.Host {
				t.Errorf("expected Host %s, got %s", tt.expected.Host, result.Host)
			}
			if result.Port != tt.expected.Port {
				t.Errorf("expected Port %d, got %d", tt.expected.Port, result.Port)
			}
			if result.User != tt.expected.User {
				t.Errorf("expected User %s, got %s", tt.expected.User, result.User)
			}
		})
	}
}

func TestNewEndpoint(t *testing.T) {
	tests := []struct {
		name          string
		host          string
		port          int
		fallbackHosts []string
		expected      Endpoint
	}{
		{
			name: "basic endpoint",
			host: "example.com",
			port: 8080,
			expected: Endpoint{
				Host: "example.com",
				Port: 8080,
				User: "",
			},
		},
		{
			name:          "empty host with fallback",
			host:          "",
			port:          3000,
			fallbackHosts: []string{"fallback.com"},
			expected: Endpoint{
				Host: "fallback.com",
				Port: 3000,
				User: "",
			},
		},
		{
			name:          "empty host with multiple fallbacks",
			host:          "",
			port:          5432,
			fallbackHosts: []string{"first.com", "second.com"},
			expected: Endpoint{
				Host: "first.com",
				Port: 5432,
				User: "",
			},
		},
		{
			name: "host with user",
			host: "user@example.com",
			port: 22,
			expected: Endpoint{
				Host: "example.com",
				Port: 22,
				User: "user",
			},
		},
		{
			name: "host with user and port in hostname",
			host: "user@example.com:9000",
			port: 22, // This should be overridden by the port in the hostname
			expected: Endpoint{
				Host: "example.com",
				Port: 9000,
				User: "user",
			},
		},
		{
			name:          "empty host no fallback",
			host:          "",
			port:          8080,
			fallbackHosts: []string{},
			expected: Endpoint{
				Host: "",
				Port: 8080,
				User: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewEndpoint(tt.host, tt.port, tt.fallbackHosts...)

			if result.Host != tt.expected.Host {
				t.Errorf("expected Host %s, got %s", tt.expected.Host, result.Host)
			}
			if result.Port != tt.expected.Port {
				t.Errorf("expected Port %d, got %d", tt.expected.Port, result.Port)
			}
			if result.User != tt.expected.User {
				t.Errorf("expected User %s, got %s", tt.expected.User, result.User)
			}
		})
	}
}

func TestEndpoint_String(t *testing.T) {
	tests := []struct {
		name     string
		endpoint Endpoint
		expected string
	}{
		{
			name: "basic endpoint",
			endpoint: Endpoint{
				Host: "example.com",
				Port: 8080,
			},
			expected: "example.com:8080",
		},
		{
			name: "endpoint with user",
			endpoint: Endpoint{
				Host: "example.com",
				Port: 22,
				User: "user",
			},
			expected: "example.com:22",
		},
		{
			name: "endpoint with zero port",
			endpoint: Endpoint{
				Host: "example.com",
				Port: 0,
			},
			expected: "example.com:0",
		},
		{
			name: "localhost endpoint",
			endpoint: Endpoint{
				Host: "localhost",
				Port: 3000,
			},
			expected: "localhost:3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.endpoint.String()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestEndpointUserParsing(t *testing.T) {
	// Test edge cases for user parsing
	tests := []struct {
		name     string
		input    string
		expected Endpoint
	}{
		{
			name:  "empty string",
			input: "",
			expected: Endpoint{
				Host: "",
				Port: 0,
				User: "",
			},
		},
		{
			name:  "just @",
			input: "@",
			expected: Endpoint{
				Host: "",
				Port: 0,
				User: "",
			},
		},
		{
			name:  "user with empty host",
			input: "user@",
			expected: Endpoint{
				Host: "",
				Port: 0,
				User: "user",
			},
		},
		{
			name:  "multiple @ symbols",
			input: "user@host@domain.com",
			expected: Endpoint{
				Host: "host", // Takes only the first part after the first @
				Port: 0,
				User: "user",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewEndpointFromString(tt.input)

			if result.Host != tt.expected.Host {
				t.Errorf("expected Host %s, got %s", tt.expected.Host, result.Host)
			}
			if result.Port != tt.expected.Port {
				t.Errorf("expected Port %d, got %d", tt.expected.Port, result.Port)
			}
			if result.User != tt.expected.User {
				t.Errorf("expected User %s, got %s", tt.expected.User, result.User)
			}
		})
	}
}

func TestIsConnectionError(t *testing.T) {
	tunnel := &Tunnel{Config: config.TunnelConfig{Name: "test"}}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"broken pipe", errors.New("broken pipe"), true},
		{"network unreachable", errors.New("network is unreachable"), true},
		{"no route to host", errors.New("no route to host"), true},
		{"timeout", errors.New("i/o timeout"), true},
		{"connection timed out", errors.New("connection timed out"), true},
		{"ssh disconnect", errors.New("ssh: disconnect"), true},
		{"ssh connection lost", errors.New("ssh: connection lost"), true},
		{"closed network connection", errors.New("use of closed network connection"), true},
		{"other error", errors.New("something else"), false},
		{"empty string", errors.New(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tunnel.isConnectionError(tt.err)
			if got != tt.want {
				t.Errorf("isConnectionError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes float64
		want  string
	}{
		{0, "0.0 B/s"},
		{100, "100.0 B/s"},
		{1024, "1.0 KB/s"},
		{1536, "1.5 KB/s"},
		{1024 * 1024, "1.0 MB/s"},
		{1024 * 1024 * 1024, "1.0 GB/s"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("formatBytes(%v) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestFormatLatency(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "n/a"},
		{-1, "n/a"},
		{time.Millisecond, "1ms"},
		{50 * time.Millisecond, "50ms"},
		{time.Second, "1000ms"},
	}
	for _, tt := range tests {
		got := formatLatency(tt.d)
		if got != tt.want {
			t.Errorf("formatLatency(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFigureOutRemoteVsBastion(t *testing.T) {
	makeCfg := func(bastionHost string, bastionPort int, remoteHost string, remotePort int) config.TunnelConfig {
		cfg := config.TunnelConfig{RemoteHost: remoteHost, RemotePort: remotePort}
		cfg.Bastion.Host = bastionHost
		cfg.Bastion.Port = bastionPort
		return cfg
	}
	tests := []struct {
		name           string
		cfg            config.TunnelConfig
		wantSSHHost    string
		wantSSHPort    int
		wantRemoteHost string
		wantRemotePort int
	}{
		{
			name:           "bastion mode",
			cfg:            makeCfg("jump.example.com", 22, "db.internal", 5432),
			wantSSHHost:    "jump.example.com", wantSSHPort: 22,
			wantRemoteHost: "db.internal", wantRemotePort: 5432,
		},
		{
			name:           "no bastion - direct remote",
			cfg:            config.TunnelConfig{RemoteHost: "svc.local", RemotePort: 8080},
			wantSSHHost:    "svc.local", wantSSHPort: 22,
			wantRemoteHost: "localhost", wantRemotePort: 8080,
		},
		{
			name:           "bastion with custom port",
			cfg:            makeCfg("jump", 2222, "db", 5432),
			wantSSHHost:    "jump", wantSSHPort: 2222,
			wantRemoteHost: "db", wantRemotePort: 5432,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sshEp, remoteEp := figureOutRemoteVsBastion(tt.cfg)
			if sshEp.Host != tt.wantSSHHost || sshEp.Port != tt.wantSSHPort {
				t.Errorf("SSH endpoint = %s:%d, want %s:%d", sshEp.Host, sshEp.Port, tt.wantSSHHost, tt.wantSSHPort)
			}
			if remoteEp.Host != tt.wantRemoteHost || remoteEp.Port != tt.wantRemotePort {
				t.Errorf("remote endpoint = %s:%d, want %s:%d", remoteEp.Host, remoteEp.Port, tt.wantRemoteHost, tt.wantRemotePort)
			}
		})
	}
}

func TestResolveIAPUser(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.TunnelConfig
		want string
	}{
		{
			name: "Bastion.User set returns as-is",
			cfg:  func() config.TunnelConfig { c := config.TunnelConfig{}; c.Bastion.User = "myuser"; return c }(),
			want: "myuser",
		},
		{
			name: "GcpIap nil returns empty",
			cfg:  config.TunnelConfig{},
			want: "",
		},
		{
			name: "GcpIap set but Bastion.User set returns Bastion.User",
			cfg:  func() config.TunnelConfig { c := config.TunnelConfig{GcpIap: &config.GcpIapConfig{}}; c.Bastion.User = "override"; return c }(),
			want: "override",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveIAPUser(tt.cfg)
			if err != nil {
				t.Errorf("ResolveIAPUser: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ResolveIAPUser() = %q, want %q", got, tt.want)
			}
		})
	}
}
