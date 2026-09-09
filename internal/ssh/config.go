package ssh

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sio2boss/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func loadPrivateKey(t *Tunnel, keyPath string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.logf("Failed to find key at %s", keyPath)
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		t.logf("failed to parse private key: %v", err)
		return nil, err
	}

	return ssh.PublicKeys(signer), nil
}

type knownHosts struct {
	file string
}

func (k *knownHosts) Callback(t *Tunnel) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		// Parse known_hosts file
		file, err := os.Open(k.file)
		if err != nil {
			// If known_hosts doesn't exist, warn but allow connection
			t.logf("Warning: %s not found, accepting host key", k.file)
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || line[0] == '#' {
				continue
			}

			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}

			// Check if hostname matches
			hostPattern := fields[0]
			if hostPattern == hostname || hostPattern == "*" || strings.HasPrefix(hostPattern, "*.") {
				// Compare key type
				keyType := fields[1]

				if key.Type() == keyType {
					expectedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
					if err == nil {
						if bytes.Equal(key.Marshal(), expectedKey.Marshal()) {
							return nil
						}
					}
				}
			}
		}

		// Host not found in known_hosts, warn and allow
		t.logf("Warning: %s not found in known_hosts, accepting host key", hostname)
		return nil
	}
}

// expandHome resolves a leading "~" the way ssh(1) does.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// agentAuthMethod returns an auth method backed by the running ssh-agent, or
// nil when there is no agent to talk to.
func agentAuthMethod(t *Tunnel) ssh.AuthMethod {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.logf("ssh-agent at %s not reachable: %v", sock, err)
		return nil
	}
	ag := agent.NewClient(conn)
	keys, err := ag.List()
	if err != nil {
		t.logf("ssh-agent gave no keys: %v", err)
		return nil
	}
	t.logf("Using ssh-agent with %d key(s)", len(keys))
	return ssh.PublicKeysCallback(ag.Signers)
}

func GetSSHConfig(t *Tunnel) (*ssh.ClientConfig, error) {
	// Find home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Try ECDSA first, then RSA
	keyPaths := []string{
		filepath.Join(home, ".ssh", "id_ecdsa"),
		filepath.Join(home, ".ssh", "id_rsa"),
	}

	// Set User: for IAP tunnels derive from the gcloud account; otherwise use config or $USER
	sshUser := t.Config.Bastion.User
	if sshUser == "" && t.Config.GcpIap != nil {
		resolved, err := ResolveIAPUser(t.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve IAP SSH user: %w", err)
		}
		sshUser = resolved
		t.logf("Resolved IAP SSH user from gcloud account: %s", sshUser)
	}
	if sshUser == "" {
		sshUser = os.Getenv("USER")
	}

	// We will resolve this host in the SSH config file
	lookupHost := &t.Config.Bastion.Host
	// The Port from the SSH config is the port we SSH to, never the port of the
	// service being forwarded. With no bastion, figureOutRemoteVsBastion reads
	// the SSH port from Bastion.Port (defaulting to 22) and forwards to
	// localhost:RemotePort, so this must land on Bastion.Port either way --
	// pointing it at RemotePort would send every tunnel to the remote's sshd.
	lookupPort := &t.Config.Bastion.Port
	if t.Config.Bastion.Host == "" {
		lookupHost = &t.Config.RemoteHost
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

			// override port with that in the SSH config
			if port, _ := sshConfig.Get(*lookupHost, "Port"); port != "" {
				if portNum, err := strconv.Atoi(port); err == nil {
					t.logf("Overriding port %d with %d from SSH config", *lookupPort, portNum)
					*lookupPort = portNum
				}
			}

			// Override Bastions User with User from SSH config
			if user, _ := sshConfig.Get(*lookupHost, "User"); user != "" {
				t.logf("Overriding user with %s from SSH config", user)
				sshUser = user
			}

			// Add identity file to auths. The SSH config keeps these paths as
			// written, so "~/.ssh/id_ed25519" arrives literally and os.ReadFile
			// would never find it.
			if identityFiles, _ := sshConfig.GetAll(*lookupHost, "IdentityFile"); len(identityFiles) > 0 {
				t.logf("Overriding identity with %d files from SSH config", len(identityFiles))
				keyPaths = nil
				for _, p := range identityFiles {
					keyPaths = append(keyPaths, expandHome(p, home))
				}
			}

			// override lookupHost with HostName from SSH config
			if host, _ := sshConfig.Get(*lookupHost, "HostName"); host != "" {
				t.logf("Overriding host %s with %s from SSH config", *lookupHost, host)
				*lookupHost = host
			}
		}
	}

	// Load Keys
	var auths []ssh.AuthMethod
	for _, keyPath := range keyPaths {
		if auth, err := loadPrivateKey(t, keyPath); err == nil {
			t.logf("Loaded identity file: %s", keyPath)
			auths = append(auths, auth)
		}
	}

	// An encrypted or absent key file is normal when the user works through an
	// agent; without this the handshake offers no method at all and fails.
	if agentAuth := agentAuthMethod(t); agentAuth != nil {
		auths = append(auths, agentAuth)
	}

	config := &ssh.ClientConfig{
		User:            sshUser,
		Auth:            auths,
		HostKeyCallback: (&knownHosts{file: filepath.Join(home, ".ssh", "known_hosts")}).Callback(t),
		Timeout:         10 * time.Second,
	}

	// Add keep-alive configuration
	config.Timeout = 10 * time.Second

	return config, nil
}
