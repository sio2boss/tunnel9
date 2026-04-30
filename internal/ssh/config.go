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

func (k *knownHosts) Callback() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		// Parse known_hosts file
		file, err := os.Open(k.file)
		if err != nil {
			// If known_hosts doesn't exist, warn but allow connection
			fmt.Printf("Warning: %s not found, accepting host key\n", k.file)
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
		fmt.Printf("Warning: %s not found in known_hosts, accepting host key\n", hostname)
		return nil
	}
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

	// Set User from config or environment variable
	sshUser := t.Config.Bastion.User
	if t.Config.Bastion.User == "" {
		sshUser = os.Getenv("USER")
	}

	// We will resolve this host in the SSH config file
	lookupHost := &t.Config.Bastion.Host
	lookupPort := &t.Config.Bastion.Port
	if t.Config.Bastion.Host == "" {
		lookupHost = &t.Config.RemoteHost
		lookupPort = &t.Config.RemotePort
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

			// Add identity file to auths
			if identityFiles, _ := sshConfig.GetAll(*lookupHost, "IdentityFile"); len(identityFiles) > 0 {
				t.logf("Overriding identity with %d files from SSH config", len(identityFiles))
				keyPaths = identityFiles
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

	config := &ssh.ClientConfig{
		User:            sshUser,
		Auth:            auths,
		HostKeyCallback: (&knownHosts{file: filepath.Join(home, ".ssh", "known_hosts")}).Callback(),
		Timeout:         10 * time.Second,
	}

	// Add keep-alive configuration
	config.Timeout = 10 * time.Second

	return config, nil
}
