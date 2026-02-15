package ssh

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"tunnel9/internal/config"

	"github.com/cedws/iapc/iap"
	"golang.org/x/oauth2"
)

// runGcloud executes a gcloud command and returns stdout (trimmed).
// On failure the error includes stderr so the user sees why it failed.
// Using Output() (not CombinedOutput) so informational stderr messages
// like "Your active configuration is: [...]" don't pollute the result.
func runGcloud(args ...string) (string, error) {
	cmd := exec.Command("gcloud", args...)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		if stderr == "" {
			stderr = err.Error()
		}
		return "", fmt.Errorf("%s", stderr)
	}
	return strings.TrimSpace(string(out)), nil
}

// gcloudTokenSource shells out to "gcloud auth print-access-token" with a
// specific configuration and project so the token is for that identity (project-specific auth).
type gcloudTokenSource struct {
	configuration string // gcloud config name (e.g. "my-project")
	project       string
	mu            sync.Mutex
	token         *oauth2.Token
}

func (g *gcloudTokenSource) Token() (*oauth2.Token, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Reuse if not expired (with 1min buffer)
	if g.token != nil && g.token.Valid() && time.Until(g.token.Expiry) > time.Minute {
		return g.token, nil
	}

	args := []string{}
	if g.configuration != "" {
		args = append(args, "--configuration="+g.configuration)
	}
	args = append(args, "auth", "print-access-token")
	if g.project != "" {
		args = append(args, "--project="+g.project)
	}
	accessToken, err := runGcloud(args...)
	if err != nil {
		return nil, fmt.Errorf("gcloud auth print-access-token failed: %w (hint: run 'gcloud --configuration=%s auth login')", err, g.configuration)
	}
	if accessToken == "" {
		return nil, fmt.Errorf("gcloud returned empty access token (hint: run 'gcloud --configuration=%s auth login')", g.configuration)
	}

	g.token = &oauth2.Token{
		AccessToken: accessToken,
		Expiry:      time.Now().Add(30 * time.Minute),
	}
	return g.token, nil
}

// resolveGcloudAccount queries the gcloud configuration for its active account email.
func resolveGcloudAccount(configuration string) (string, error) {
	args := []string{}
	if configuration != "" {
		args = append(args, "--configuration="+configuration)
	}
	args = append(args, "config", "get-value", "account")
	account, err := runGcloud(args...)
	if err != nil {
		return "", fmt.Errorf("failed to resolve gcloud account for configuration %q: %w", configuration, err)
	}
	if account == "" || account == "(unset)" {
		return "", fmt.Errorf("no account set in gcloud configuration %q (run 'gcloud --configuration=%s auth login')", configuration, configuration)
	}
	return account, nil
}

// ResolveIAPUser returns the SSH username for an IAP tunnel by querying the
// gcloud configuration's active account and extracting the email prefix.
// If Bastion.User is already set it is returned as-is.
func ResolveIAPUser(cfg config.TunnelConfig) (string, error) {
	if cfg.Bastion.User != "" {
		return cfg.Bastion.User, nil
	}
	if cfg.GcpIap == nil {
		return "", nil
	}
	configuration := strings.TrimSpace(cfg.GcpIap.GcloudConfiguration)
	if configuration == "" {
		configuration = cfg.GcpIap.Project
	}
	account, err := resolveGcloudAccount(configuration)
	if err != nil {
		return "", err
	}
	// SSH user is the local-part of the email, with dots/plus replaced by underscores
	// (matches gcloud compute ssh behaviour)
	user := account
	if at := strings.Index(account, "@"); at > 0 {
		user = account[:at]
	}
	user = strings.NewReplacer(".", "_", "+", "_").Replace(user)
	return user, nil
}

// dialIAP establishes a TCP connection to the given instance's SSH port through GCP IAP.
// Uses gcloud CLI auth which respects per-project credentials.
// The tunnel parameter is used for logging; pass nil to skip logging.
func dialIAP(ctx context.Context, cfg config.TunnelConfig, t *Tunnel) (net.Conn, error) {
	if cfg.GcpIap == nil || cfg.Bastion.Host == "" {
		return nil, nil
	}

	project := cfg.GcpIap.Project
	zone := cfg.GcpIap.Zone
	instance := cfg.Bastion.Host
	ninterface := cfg.GcpIap.Interface
	if ninterface == "" {
		ninterface = "nic0"
	}
	port := cfg.Bastion.Port
	if port == 0 {
		port = 22
	}
	if strings.TrimSpace(cfg.GcpIap.Mode) == "direct" {
		port = cfg.RemotePort
		if port == 0 {
			return nil, fmt.Errorf("IAP direct mode requires remote_port")
		}
	}

	if project == "" {
		return nil, fmt.Errorf("IAP requires gcp_iap.project (the GCP project for the tunnel)")
	}
	configuration := strings.TrimSpace(cfg.GcpIap.GcloudConfiguration)
	if configuration == "" {
		configuration = project
	}

	// Verify the gcloud account before dialing so problems are visible early
	account, err := resolveGcloudAccount(configuration)
	if err != nil {
		return nil, err
	}

	// Log the values we're using so problems are visible in the console
	if t != nil {
		t.logf("IAP dial: gcloud_config=%s account=%s project=%s zone=%s instance=%s port=%d interface=%s",
			configuration, account, project, zone, instance, port, ninterface)
	}

	gts := &gcloudTokenSource{configuration: configuration, project: project}
	if _, err := gts.Token(); err != nil {
		return nil, err
	}
	var tokenSource oauth2.TokenSource = gts

	opts := []iap.DialOption{
		iap.WithProject(project),
		iap.WithInstance(instance, zone, ninterface),
		iap.WithPort(strconv.Itoa(port)),
		iap.WithTokenSource(&tokenSource),
	}

	conn, err := iap.Dial(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
