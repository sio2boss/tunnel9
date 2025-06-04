package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTunnelConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   TunnelConfig
		expected TunnelConfig
	}{
		{
			name: "basic tunnel config",
			config: TunnelConfig{
				Name:       "test-tunnel",
				LocalPort:  8080,
				RemotePort: 3000,
				RemoteHost: "example.com",
				Tag:        "test",
			},
			expected: TunnelConfig{
				Name:       "test-tunnel",
				LocalPort:  8080,
				RemotePort: 3000,
				RemoteHost: "example.com",
				Tag:        "test",
			},
		},
		{
			name: "tunnel config with bastion",
			config: TunnelConfig{
				Name:       "bastion-tunnel",
				LocalPort:  5432,
				RemotePort: 5432,
				RemoteHost: "db.internal",
				Tag:        "production",
				Bastion: struct {
					Host string `yaml:"host"`
					User string `yaml:"user"`
					Port int    `yaml:"port,omitempty"`
				}{
					Host: "jump.server.com",
					User: "jumpuser",
					Port: 22,
				},
			},
			expected: TunnelConfig{
				Name:       "bastion-tunnel",
				LocalPort:  5432,
				RemotePort: 5432,
				RemoteHost: "db.internal",
				Tag:        "production",
				Bastion: struct {
					Host string `yaml:"host"`
					User string `yaml:"user"`
					Port int    `yaml:"port,omitempty"`
				}{
					Host: "jump.server.com",
					User: "jumpuser",
					Port: 22,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.Name != tt.expected.Name {
				t.Errorf("expected Name %s, got %s", tt.expected.Name, tt.config.Name)
			}
			if tt.config.LocalPort != tt.expected.LocalPort {
				t.Errorf("expected LocalPort %d, got %d", tt.expected.LocalPort, tt.config.LocalPort)
			}
		})
	}
}

func TestConfigLoader_Load(t *testing.T) {
	// Create a temporary directory for test configs
	tempDir, err := os.MkdirTemp("", "tunnel9-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name        string
		configYAML  string
		expectedLen int
		expectError bool
	}{
		{
			name: "valid config with multiple tunnels",
			configYAML: `tunnels:
  - name: "web-tunnel"
    local_port: 8080
    remote_port: 80
    remote_host: "web.example.com"
    tag: "web"
  - name: "db-tunnel"
    local_port: 5432
    remote_port: 5432
    remote_host: "db.example.com"
    tag: "database"
    bastion:
      host: "jump.example.com"
      user: "jumpuser"
      port: 22`,
			expectedLen: 2,
			expectError: false,
		},
		{
			name:        "empty config",
			configYAML:  `tunnels: []`,
			expectedLen: 0,
			expectError: false,
		},
		{
			name: "invalid yaml",
			configYAML: `tunnels:
  - name: "invalid
    local_port: not_a_number`,
			expectedLen: 0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test config file
			configPath := filepath.Join(tempDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.configYAML), 0644)
			if err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			// Test loading
			loader := NewConfigLoader(configPath)
			tunnels, err := loader.Load()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(tunnels) != tt.expectedLen {
				t.Errorf("expected %d tunnels, got %d", tt.expectedLen, len(tunnels))
			}
		})
	}
}

func TestConfigLoader_Save(t *testing.T) {
	// Create a temporary directory for test configs
	tempDir, err := os.MkdirTemp("", "tunnel9-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tunnels := []TunnelConfig{
		{
			Name:       "test-tunnel",
			LocalPort:  8080,
			RemotePort: 3000,
			RemoteHost: "test.example.com",
			Tag:        "test",
		},
		{
			Name:       "db-tunnel",
			LocalPort:  5432,
			RemotePort: 5432,
			RemoteHost: "db.example.com",
			Tag:        "database",
			Bastion: struct {
				Host string `yaml:"host"`
				User string `yaml:"user"`
				Port int    `yaml:"port,omitempty"`
			}{
				Host: "jump.example.com",
				User: "jumpuser",
				Port: 22,
			},
		},
	}

	configPath := filepath.Join(tempDir, "config.yaml")
	loader := NewConfigLoader(configPath)

	// Test saving
	err = loader.Save(tunnels)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Test loading back
	loadedTunnels, err := loader.Load()
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if len(loadedTunnels) != len(tunnels) {
		t.Errorf("expected %d tunnels, got %d", len(tunnels), len(loadedTunnels))
	}

	// Check first tunnel
	if len(loadedTunnels) > 0 {
		if loadedTunnels[0].Name != tunnels[0].Name {
			t.Errorf("expected name %s, got %s", tunnels[0].Name, loadedTunnels[0].Name)
		}
		if loadedTunnels[0].LocalPort != tunnels[0].LocalPort {
			t.Errorf("expected local port %d, got %d", tunnels[0].LocalPort, loadedTunnels[0].LocalPort)
		}
	}
}

func TestConfigLoader_LoadNonExistentFile(t *testing.T) {
	// Test loading a non-existent file
	loader := NewConfigLoader("/non/existent/path/config.yaml")
	_, err := loader.Load()

	if err == nil {
		t.Error("expected error when loading non-existent file")
	}
}

func TestGetDefaultConfigPath(t *testing.T) {
	path := GetDefaultConfigPath()

	if path == "" {
		t.Error("default config path should not be empty")
	}

	// Should contain expected structure
	expectedParts := []string{".local", "state", "tunnel9", "config.yaml"}
	for _, part := range expectedParts {
		if !containsPathPart(path, part) {
			t.Errorf("default config path should contain %s, got %s", part, path)
		}
	}
}

// Helper function to check if path contains a specific part
func containsPathPart(path, part string) bool {
	// Simple string contains check for path components
	return filepath.Base(path) == part ||
		filepath.Dir(path) != "." && containsPathPart(filepath.Dir(path), part)
}
