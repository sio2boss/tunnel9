package ssh

import (
	"testing"
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
