package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"tunnel9/internal/config"
)

// writeSSHHome builds a throwaway $HOME containing .ssh/config, and returns it.
func writeSSHHome(t *testing.T, sshConfig string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(sshConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	return home
}

func newTestTunnel(t *testing.T, tc config.TunnelConfig) *Tunnel {
	t.Helper()
	tun := &Tunnel{ID: "test", Config: tc, LogChan: make(chan string, 100)}
	// Drain the log channel so logf never blocks on the buffer.
	go func() {
		for range tun.LogChan {
		}
	}()
	t.Cleanup(func() { close(tun.LogChan) })
	return tun
}

// The Port in the SSH config is the port we SSH to. Writing it into
// RemotePort would point every forward at the remote's sshd instead of at the
// service the user asked for.
func TestGetSSHConfigKeepsRemotePort(t *testing.T) {
	writeSSHHome(t, "Host dev\n    HostName dev.example.com\n    Port 2222\n")

	tun := newTestTunnel(t, config.TunnelConfig{
		Name:       "postgres",
		RemoteHost: "dev",
		LocalPort:  15432,
		RemotePort: 5432,
	})
	if _, err := GetSSHConfig(tun); err != nil {
		t.Fatalf("GetSSHConfig: %v", err)
	}

	if tun.Config.RemotePort != 5432 {
		t.Errorf("RemotePort = %d, want 5432 (the service port must survive)", tun.Config.RemotePort)
	}
	if tun.Config.Bastion.Port != 2222 {
		t.Errorf("Bastion.Port = %d, want 2222 (the SSH port belongs here)", tun.Config.Bastion.Port)
	}

	sshEndpoint, remoteEndpoint := figureOutRemoteVsBastion(tun.Config)
	if got, want := sshEndpoint.String(), "dev.example.com:2222"; got != want {
		t.Errorf("ssh endpoint = %s, want %s", got, want)
	}
	if got, want := remoteEndpoint.String(), "localhost:5432"; got != want {
		t.Errorf("remote endpoint = %s, want %s", got, want)
	}
}

func TestExpandHome(t *testing.T) {
	tests := []struct {
		name, path, want string
	}{
		{"tilde slash", "~/.ssh/id_ed25519", "/home/u/.ssh/id_ed25519"},
		{"bare tilde", "~", "/home/u"},
		{"absolute path untouched", "/etc/ssh/key", "/etc/ssh/key"},
		{"relative path untouched", "keys/id", "keys/id"},
		{"tilde user not expanded", "~other/.ssh/id", "~other/.ssh/id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandHome(tt.path, "/home/u"); got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// An IdentityFile is stored as written, so "~/.ssh/id_ed25519" reaches us
// literally and os.ReadFile would never find it.
func TestIdentityFileTildeIsExpanded(t *testing.T) {
	home := writeSSHHome(t, "Host dev\n    HostName dev.example.com\n    IdentityFile ~/.ssh/id_test\n")

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(home, ".ssh", "id_test")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}

	tun := newTestTunnel(t, config.TunnelConfig{Name: "svc", RemoteHost: "dev", RemotePort: 5432})
	clientConfig, err := GetSSHConfig(tun)
	if err != nil {
		t.Fatalf("GetSSHConfig: %v", err)
	}
	if len(clientConfig.Auth) == 0 {
		t.Fatal("no auth methods: the tilde in IdentityFile was not expanded")
	}
}
