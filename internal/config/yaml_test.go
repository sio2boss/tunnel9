package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	path, err := GetDefaultConfigPath()
	if err != nil {
		t.Fatalf("GetDefaultConfigPath: %v", err)
	}
	if path == "" {
		t.Error("default config path should not be empty")
	}
	expectedParts := []string{".local", "state", "tunnel9", "config.yaml"}
	for _, part := range expectedParts {
		if !containsPathPart(path, part) {
			t.Errorf("default config path should contain %s, got %s", part, path)
		}
	}
}

func TestGetDefaultConfigPath_HomeDirError(t *testing.T) {
	old := userHomeDir
	defer func() { userHomeDir = old }()
	userHomeDir = func() (string, error) {
		return "", errors.New("injected home dir error")
	}
	path, err := GetDefaultConfigPath()
	if err == nil {
		t.Errorf("GetDefaultConfigPath expected error, got path %q", path)
	}
	if path != "" {
		t.Errorf("GetDefaultConfigPath on error should return empty path, got %q", path)
	}
}

// Helper function to check if path contains a specific part
func containsPathPart(path, part string) bool {
	// Simple string contains check for path components
	return filepath.Base(path) == part ||
		filepath.Dir(path) != "." && containsPathPart(filepath.Dir(path), part)
}

func TestFindConfigFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tunnel9-findconfig-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Explicit path that exists
	explicitPath := filepath.Join(tempDir, "custom.yaml")
	if err := os.WriteFile(explicitPath, []byte("tunnels: []"), 0644); err != nil {
		t.Fatalf("failed to write explicit config: %v", err)
	}
	got, err := FindConfigFile(explicitPath)
	if err != nil {
		t.Fatalf("FindConfigFile(existing path): %v", err)
	}
	if got != explicitPath {
		t.Errorf("FindConfigFile(existing path) = %q, want %q", got, explicitPath)
	}

	// 2. Explicit path that does not exist: should fall back (default path)
	got, err = FindConfigFile(filepath.Join(tempDir, "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("FindConfigFile(nonexistent): %v", err)
	}
	if got == "" {
		t.Error("FindConfigFile(nonexistent) should not return empty")
	}
	if !containsPathPart(got, "config.yaml") {
		t.Errorf("fallback should point at config.yaml, got %q", got)
	}

	// 3. Empty string: look for .tunnel9.yaml in cwd, then fallback
	localConfig := filepath.Join(tempDir, ".tunnel9.yaml")
	if err := os.WriteFile(localConfig, []byte("tunnels: []"), 0644); err != nil {
		t.Fatalf("failed to write .tunnel9.yaml: %v", err)
	}
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origWd)
	got, err = FindConfigFile("")
	if err != nil {
		t.Fatalf("FindConfigFile(\"\") with .tunnel9.yaml in cwd: %v", err)
	}
	gotResolved, _ := filepath.EvalSymlinks(got)
	wantResolved, _ := filepath.EvalSymlinks(localConfig)
	if gotResolved != wantResolved {
		t.Errorf("FindConfigFile(\"\") with .tunnel9.yaml in cwd = %q, want %q", got, localConfig)
	}
}

func TestFindConfigFile_HomeDirError(t *testing.T) {
	old := userHomeDir
	defer func() { userHomeDir = old }()
	userHomeDir = func() (string, error) {
		return "", errors.New("injected")
	}
	tempDir, err := os.MkdirTemp("", "tunnel9-findconfig-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	origWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(origWd)
	// No .tunnel9.yaml, so FindConfigFile falls back to GetDefaultConfigPath which will fail
	_, err = FindConfigFile("")
	if err == nil {
		t.Error("FindConfigFile(\"\") expected error when home dir fails")
	}
}

func TestConfigLoader_SaveCreatesDirectory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tunnel9-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save to a path under a subdirectory that does not exist yet
	configPath := filepath.Join(tempDir, "subdir", "nested", "config.yaml")
	loader := NewConfigLoader(configPath)
	tunnels := []TunnelConfig{
		{Name: "one", LocalPort: 8080, RemotePort: 80, RemoteHost: "host", Tag: "x"},
	}
	if err := loader.Save(tunnels); err != nil {
		t.Fatalf("Save to new directory failed: %v", err)
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created under new directory")
	}
	loaded, err := loader.Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Name != "one" {
		t.Errorf("loaded config mismatch: got %v", loaded)
	}
}

func TestConfigLoader_Save_MkdirAllFails(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tunnel9-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	// Create a file (not a directory); saving under it will make MkdirAll fail
	blocker := filepath.Join(tempDir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to create blocker file: %v", err)
	}
	configPath := filepath.Join(blocker, "sub", "config.yaml")
	loader := NewConfigLoader(configPath)
	err = loader.Save([]TunnelConfig{{Name: "x", LocalPort: 1, RemotePort: 2, RemoteHost: "h", Tag: "t"}})
	if err == nil {
		t.Error("Save expected error when parent is a file")
	}
	if err != nil && !strings.Contains(err.Error(), "creating config directory") {
		t.Errorf("Save error should mention creating config directory: %v", err)
	}
}

func TestConfigLoader_Save_WriteFileFails(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tunnel9-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	// Save to a path that is an existing directory; WriteFile will fail
	loader := NewConfigLoader(tempDir)
	err = loader.Save([]TunnelConfig{{Name: "x", LocalPort: 1, RemotePort: 2, RemoteHost: "h", Tag: "t"}})
	if err == nil {
		t.Error("Save expected error when path is a directory")
	}
	if err != nil && !strings.Contains(err.Error(), "writing config") {
		t.Errorf("Save error should mention writing config: %v", err)
	}
}

func TestConfigLoader_Save_MarshalFails(t *testing.T) {
	old := yamlMarshal
	defer func() { yamlMarshal = old }()
	yamlMarshal = func(interface{}) ([]byte, error) {
		return nil, errors.New("injected marshal error")
	}
	tempDir, err := os.MkdirTemp("", "tunnel9-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	loader := NewConfigLoader(filepath.Join(tempDir, "config.yaml"))
	err = loader.Save([]TunnelConfig{{Name: "x", LocalPort: 1, RemotePort: 2, RemoteHost: "h", Tag: "t"}})
	if err == nil {
		t.Error("Save expected error when marshal fails")
	}
	if err != nil && !strings.Contains(err.Error(), "marshaling config") {
		t.Errorf("Save error should mention marshaling config: %v", err)
	}
}
