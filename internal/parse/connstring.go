package parse

import (
	"fmt"
	"strconv"
	"strings"

	"tunnel9/internal/config"
)

// ParseSshString parses an SSH -N -L connection string into a tunnel config.
func ParseSshString(sshStr string) (*config.TunnelConfig, error) {
	parts := strings.Fields(sshStr)
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid ssh string format")
	}

	var portMapping string
	for i, part := range parts {
		if part == "-L" && i+1 < len(parts) {
			portMapping = parts[i+1]
			break
		}
	}

	if portMapping == "" {
		return nil, fmt.Errorf("no port mapping (-L) found")
	}

	portParts := strings.Split(portMapping, ":")
	var localPort int
	var remoteHost string
	var remotePort int
	var bindAddr string
	var err error

	switch len(portParts) {
	case 4:
		bindAddr = portParts[0]
		localPort, err = strconv.Atoi(portParts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid local port: %v", err)
		}
		remoteHost = portParts[2]
		remotePort, err = strconv.Atoi(portParts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid remote port: %v", err)
		}
	case 3:
		localPort, err = strconv.Atoi(portParts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid local port: %v", err)
		}
		remoteHost = portParts[1]
		remotePort, err = strconv.Atoi(portParts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid remote port: %v", err)
		}
	default:
		return nil, fmt.Errorf("invalid port mapping format")
	}

	if remoteHost == "" {
		return nil, fmt.Errorf("remote host cannot be empty")
	}

	cfg := config.TunnelConfig{
		Name:        fmt.Sprintf("%s-%d", remoteHost, localPort),
		LocalPort:   localPort,
		RemotePort:  remotePort,
		RemoteHost:  remoteHost,
		BindAddress: bindAddr,
	}

	lastArg := parts[len(parts)-1]
	if !strings.HasPrefix(lastArg, "-") {
		if !strings.Contains(lastArg, "@") {
			cfg.Bastion.Host = lastArg
		} else {
			userHostParts := strings.Split(lastArg, "@")
			if len(userHostParts) == 2 {
				cfg.Bastion.User = userHostParts[0]
				hostParts := strings.Split(userHostParts[1], ":")
				if len(hostParts) == 2 {
					cfg.Bastion.Host = hostParts[0]
					port, err := strconv.Atoi(hostParts[1])
					if err == nil {
						cfg.Bastion.Port = port
					}
				} else {
					cfg.Bastion.Host = userHostParts[1]
				}
			}
		}
		if cfg.Bastion.Port == 0 {
			cfg.Bastion.Port = 22
		}
	}

	return &cfg, nil
}

// ParseGcloudSshString parses a gcloud compute ssh --tunnel-through-iap command string.
// Returns nil, nil if the string does not look like a gcloud IAP SSH command.
func ParseGcloudSshString(s string) (*config.TunnelConfig, error) {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if !strings.Contains(s, "gcloud") || !strings.Contains(s, "compute") ||
		!strings.Contains(s, "ssh") || !strings.Contains(s, "tunnel-through-iap") {
		return nil, nil
	}

	var zone string
	if i := strings.Index(s, "--zone="); i >= 0 {
		i += len("--zone=")
		end := i
		for end < len(s) && s[end] != ' ' && s[end] != '\t' && s[end] != '\n' {
			end++
		}
		zone = strings.TrimSpace(s[i:end])
	}
	if zone == "" {
		return nil, fmt.Errorf("gcloud IAP command requires --zone=...")
	}

	var project string
	if i := strings.Index(s, "--project="); i >= 0 {
		i += len("--project=")
		end := i
		for end < len(s) && s[end] != ' ' && s[end] != '\t' && s[end] != '\n' {
			end++
		}
		project = strings.TrimSpace(s[i:end])
	}

	var gcloudConfig string
	if i := strings.Index(s, "--configuration="); i >= 0 {
		i += len("--configuration=")
		end := i
		for end < len(s) && s[end] != ' ' && s[end] != '\t' && s[end] != '\n' {
			end++
		}
		gcloudConfig = strings.TrimSpace(s[i:end])
	}

	parts := strings.Fields(s)
	var instance string
	for i := 0; i < len(parts); i++ {
		if parts[i] == "ssh" && i+1 < len(parts) {
			for j := i + 1; j < len(parts); j++ {
				if !strings.HasPrefix(parts[j], "-") {
					instance = parts[j]
					break
				}
			}
			break
		}
	}
	if instance == "" {
		return nil, fmt.Errorf("gcloud IAP command: instance name not found")
	}
	bastionUser := ""
	if at := strings.Index(instance, "@"); at > 0 {
		bastionUser = instance[:at]
		instance = instance[at+1:]
	}

	var portMapping string
	if i := strings.Index(s, "--ssh-flag="); i >= 0 {
		i += len("--ssh-flag=")
		quote := byte(0)
		if i < len(s) && (s[i] == '"' || s[i] == '\'') {
			quote = s[i]
			i++
		}
		end := i
		for end < len(s) {
			if quote != 0 {
				if s[end] == quote {
					end++
					break
				}
			} else if s[end] == ' ' || s[end] == '\t' || s[end] == '\n' {
				break
			}
			end++
		}
		flagVal := s[i:end]
		if quote != 0 && len(flagVal) > 0 && flagVal[len(flagVal)-1] == quote {
			flagVal = flagVal[:len(flagVal)-1]
		}
		subParts := strings.Fields(flagVal)
		for j := 0; j < len(subParts)-1; j++ {
			if subParts[j] == "-L" {
				portMapping = subParts[j+1]
				break
			}
		}
	}
	if portMapping == "" {
		return nil, fmt.Errorf("gcloud IAP command: no -L port mapping in --ssh-flag")
	}

	portParts := strings.Split(portMapping, ":")
	var localPort int
	var remoteHost string
	var remotePort int
	var bindAddr string
	var err error
	switch len(portParts) {
	case 4:
		bindAddr = portParts[0]
		localPort, err = strconv.Atoi(portParts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid local port: %v", err)
		}
		remoteHost = portParts[2]
		remotePort, err = strconv.Atoi(portParts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid remote port: %v", err)
		}
	case 3:
		localPort, err = strconv.Atoi(portParts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid local port: %v", err)
		}
		remoteHost = portParts[1]
		remotePort, err = strconv.Atoi(portParts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid remote port: %v", err)
		}
	default:
		return nil, fmt.Errorf("invalid port mapping format")
	}
	if remoteHost == "" {
		return nil, fmt.Errorf("remote host cannot be empty")
	}

	tc := &config.TunnelConfig{
		Name:        fmt.Sprintf("%s-%d", remoteHost, localPort),
		LocalPort:   localPort,
		RemotePort:  remotePort,
		RemoteHost:  remoteHost,
		BindAddress: bindAddr,
		GcpIap: &config.GcpIapConfig{
			Zone:                zone,
			Project:             project,
			GcloudConfiguration: gcloudConfig,
		},
	}
	tc.Bastion.Host = instance
	tc.Bastion.User = bastionUser
	tc.Bastion.Port = 22
	return tc, nil
}

// ParseGcloudStartIapTunnel parses a gcloud compute start-iap-tunnel command string.
// Returns nil, nil if the string does not look like a start-iap-tunnel command.
func ParseGcloudStartIapTunnel(s string) (*config.TunnelConfig, error) {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if !strings.Contains(s, "gcloud") || !strings.Contains(s, "compute") ||
		!strings.Contains(s, "start-iap-tunnel") {
		return nil, nil
	}

	var zone string
	if i := strings.Index(s, "--zone="); i >= 0 {
		i += len("--zone=")
		end := i
		for end < len(s) && s[end] != ' ' && s[end] != '\t' && s[end] != '\n' {
			end++
		}
		zone = strings.TrimSpace(s[i:end])
	}
	if zone == "" {
		return nil, fmt.Errorf("gcloud start-iap-tunnel requires --zone=...")
	}

	var project string
	if i := strings.Index(s, "--project="); i >= 0 {
		i += len("--project=")
		end := i
		for end < len(s) && s[end] != ' ' && s[end] != '\t' && s[end] != '\n' {
			end++
		}
		project = strings.TrimSpace(s[i:end])
	}

	var gcloudConfig string
	if i := strings.Index(s, "--configuration="); i >= 0 {
		i += len("--configuration=")
		end := i
		for end < len(s) && s[end] != ' ' && s[end] != '\t' && s[end] != '\n' {
			end++
		}
		gcloudConfig = strings.TrimSpace(s[i:end])
	}

	parts := strings.Fields(s)
	var instance string
	var instancePort int
	for i := 0; i < len(parts); i++ {
		if parts[i] == "start-iap-tunnel" && i+2 <= len(parts) {
			j := i + 1
			for j < len(parts) && strings.HasPrefix(parts[j], "-") {
				j++
			}
			if j < len(parts) {
				instance = parts[j]
				j++
			}
			for j < len(parts) && strings.HasPrefix(parts[j], "-") {
				j++
			}
			if j < len(parts) {
				var err error
				instancePort, err = strconv.Atoi(parts[j])
				if err != nil {
					return nil, fmt.Errorf("gcloud start-iap-tunnel: invalid instance port %q", parts[j])
				}
			}
			break
		}
	}
	if instance == "" {
		return nil, fmt.Errorf("gcloud start-iap-tunnel: instance name not found")
	}
	if instancePort == 0 {
		return nil, fmt.Errorf("gcloud start-iap-tunnel: instance port not found")
	}

	localHost := "localhost"
	localPort := instancePort
	if i := strings.Index(s, "--local-host-port="); i >= 0 {
		i += len("--local-host-port=")
		end := i
		for end < len(s) && s[end] != ' ' && s[end] != '\t' && s[end] != '\n' {
			end++
		}
		val := strings.TrimSpace(s[i:end])
		if colon := strings.Index(val, ":"); colon >= 0 {
			localHost = strings.TrimSpace(val[:colon])
			if p, err := strconv.Atoi(strings.TrimSpace(val[colon+1:])); err == nil {
				localPort = p
			}
		} else if p, err := strconv.Atoi(val); err == nil {
			localPort = p
		}
	}

	tc := &config.TunnelConfig{
		Name:        fmt.Sprintf("%s-%d", instance, localPort),
		LocalPort:   localPort,
		RemotePort:  instancePort,
		RemoteHost:  "",
		BindAddress: localHost,
		GcpIap: &config.GcpIapConfig{
			Mode:                "direct",
			Zone:                zone,
			Project:             project,
			GcloudConfiguration: gcloudConfig,
		},
	}
	tc.Bastion.Host = instance
	return tc, nil
}
