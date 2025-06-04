package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tunnel9/internal/config"
	"tunnel9/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/docopt/docopt-go"
)

const VERSION string = "1.0.2"
const USAGE_CONTENT string = `tunnel9 - SSH Tunnel Manager

Version: %s

Usage:
  tunnel9 [--config=<path>]
  tunnel9 -h | --help

Options:
  -h --help        Show this screen.
  --config=<path>  Path to config file [default: ~/.local/state/tunnel9/config.yaml]`

func main() {
	usage := fmt.Sprintf(USAGE_CONTENT, VERSION)
	opts, err := docopt.ParseArgs(usage, os.Args[1:], VERSION)
	if err != nil {
		fmt.Println("Error parsing arguments:", err)
		os.Exit(1)
	}

	configPath := opts["--config"].(string)
	if configPath == "~/.local/state/tunnel9/config.yaml" {
		configPath = config.GetDefaultConfigPath()
	}

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Printf("Error creating config directory %s: %v\n", configDir, err)
		os.Exit(1)
	}

	// Load configuration
	loader := config.NewConfigLoader(configPath)
	tunnels, err := loader.Load()
	if err != nil {
		fmt.Println("Unable to load configuration")
		fmt.Println("  - ", err)
		fmt.Println("proceeding with empty config...")
		time.Sleep(2 * time.Second)
	}

	app := ui.NewApp(loader, tunnels)
	p := tea.NewProgram(
		app,
		tea.WithAltScreen(), // Use alternate screen buffer
	)

	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
