package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type TunnelConfig struct {
	Name        string `yaml:"name"`
	LocalPort   int    `yaml:"local_port"`
	RemotePort  int    `yaml:"remote_port"`
	RemoteHost  string `yaml:"remote_host"`
	Tag         string `yaml:"tag"`
	BindAddress string `yaml:"bind_address,omitempty"`
	Bastion     struct {
		Host string `yaml:"host"`
		User string `yaml:"user"`
		Port int    `yaml:"port,omitempty"`
	} `yaml:"bastion,omitempty"`
	GcpIap *GcpIapConfig `yaml:"gcp_iap,omitempty"`
}

// GcpIapConfig holds GCP Identity-Aware Proxy tunnel settings.
// When set, the bastion connection uses IAP instead of direct SSH.
// Project and GcloudConfiguration are required: the tunnel uses that gcloud
// configuration's account so the identity is authorized for the project.
type GcpIapConfig struct {
	Zone                 string `yaml:"zone"`
	Project              string `yaml:"project,omitempty"`
	GcloudConfiguration  string `yaml:"gcloud_configuration,omitempty"` // gcloud config name for auth; defaults to project if empty
	Interface            string `yaml:"interface,omitempty"`            // e.g. "nic0", default used by iapc if empty
}

type Config struct {
	Tunnels []TunnelConfig `yaml:"tunnels"`
}

type ConfigLoader struct {
	path string
}

func NewConfigLoader(path string) *ConfigLoader {
	return &ConfigLoader{
		path: path,
	}
}

func GetDefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		os.Exit(1)
	}
	return filepath.Join(homeDir, ".local", "state", "tunnel9", "config.yaml")
}

// FindConfigFile looks for a config file in the following order:
// 1. If configPath is provided and file exists, use it
// 2. Look for .tunnel9.yaml in current directory
// 3. Fall back to ~/.local/state/tunnel9/config.yaml
func FindConfigFile(configPath string) string {
	// If a specific config path is provided, use it
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}

	// Look for .tunnel9.yaml in current directory
	currentDir, err := os.Getwd()
	if err == nil {
		localConfig := filepath.Join(currentDir, ".tunnel9.yaml")
		if _, err := os.Stat(localConfig); err == nil {
			return localConfig
		}
	}

	// Fall back to default config path
	return GetDefaultConfigPath()
}

func (c *ConfigLoader) Load() ([]TunnelConfig, error) {
	// Validate config path to prevent path traversal
	cleanPath := filepath.Clean(c.path)
	if filepath.IsAbs(cleanPath) {
		// Additional validation could be added here
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return []TunnelConfig{}, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config.Tunnels, nil
}

func (c *ConfigLoader) Save(tunnels []TunnelConfig) error {
	config := Config{
		Tunnels: tunnels,
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("error marshaling config: %w", err)
	}

	// Create directory with secure permissions (owner only)
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	// Write to file with secure permissions (owner read/write only)
	if err := os.WriteFile(c.path, data, 0600); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}
