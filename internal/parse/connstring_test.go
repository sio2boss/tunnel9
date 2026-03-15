package parse

import (
	"strings"
	"testing"

	"tunnel9/internal/config"
)

func TestParseSshString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, tc *config.TunnelConfig)
	}{
		{
			name:    "valid 3-part port mapping without bastion",
			input:   "ssh -N -L 5432:db.internal:5432",
			wantErr: false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.LocalPort != 5432 || tc.RemoteHost != "db.internal" || tc.RemotePort != 5432 {
					t.Errorf("got LocalPort=%d RemoteHost=%s RemotePort=%d", tc.LocalPort, tc.RemoteHost, tc.RemotePort)
				}
				if tc.Name != "db.internal-5432" {
					t.Errorf("Name = %q", tc.Name)
				}
			},
		},
		{
			name:    "valid with bastion user@host",
			input:   "ssh -N -L 2345:10.0.0.5:5432 jump@bastion.example.com",
			wantErr: false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.LocalPort != 2345 || tc.RemoteHost != "10.0.0.5" || tc.RemotePort != 5432 {
					t.Errorf("got LocalPort=%d RemoteHost=%s RemotePort=%d", tc.LocalPort, tc.RemoteHost, tc.RemotePort)
				}
				if tc.Bastion.User != "jump" || tc.Bastion.Host != "bastion.example.com" {
					t.Errorf("got Bastion User=%s Host=%s", tc.Bastion.User, tc.Bastion.Host)
				}
				if tc.Bastion.Port != 22 {
					t.Errorf("expected default bastion port 22, got %d", tc.Bastion.Port)
				}
			},
		},
		{
			name:    "valid with bastion host only (host:port without user stored as host string)",
			input:   "ssh -N -L 8080:svc:80 mybastion:2222",
			wantErr: false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.Bastion.Host != "mybastion:2222" {
					t.Errorf("got Bastion Host=%s", tc.Bastion.Host)
				}
				if tc.Bastion.Port != 22 {
					t.Errorf("got Bastion Port=%d", tc.Bastion.Port)
				}
			},
		},
		{
			name:    "valid 4-part with bind address",
			input:   "ssh -N -L 0.0.0.0:3306:mysql:3306",
			wantErr: false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.BindAddress != "0.0.0.0" || tc.LocalPort != 3306 || tc.RemoteHost != "mysql" || tc.RemotePort != 3306 {
					t.Errorf("got BindAddress=%s LocalPort=%d RemoteHost=%s RemotePort=%d",
						tc.BindAddress, tc.LocalPort, tc.RemoteHost, tc.RemotePort)
				}
			},
		},
		{
			name:    "too few parts",
			input:   "ssh -N",
			wantErr: true,
		},
		{
			name:    "no -L port mapping",
			input:   "ssh -N -D 1080",
			wantErr: true,
		},
		{
			name:    "invalid port mapping format",
			input:   "ssh -N -L  onlytwo",
			wantErr: true,
		},
		{
			name:    "invalid local port",
			input:   "ssh -N -L abc:host:80",
			wantErr: true,
		},
		{
			name:    "4-part invalid local port",
			input:   "ssh -N -L 0.0.0.0:bad:host:80",
			wantErr: true,
		},
		{
			name:    "4-part invalid remote port",
			input:   "ssh -N -L 0.0.0.0:3306:mysql:nope",
			wantErr: true,
		},
		{
			name:    "3-part invalid remote port",
			input:   "ssh -N -L 3306:mysql:nope",
			wantErr: true,
		},
		{
			name:    "port mapping not 3 or 4 parts (two parts)",
			input:   "ssh -N -L 80:host",
			wantErr: true,
		},
		{
			name:    "remote host empty",
			input:   "ssh -N -L 123::80",
			wantErr: true,
		},
		{
			name:    "valid bastion user@host:port sets Bastion.Port",
			input:   "ssh -N -L 2345:10.0.0.5:5432 jump@bastion.example.com:2222",
			wantErr: false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.Bastion.User != "jump" || tc.Bastion.Host != "bastion.example.com" || tc.Bastion.Port != 2222 {
					t.Errorf("got Bastion User=%s Host=%s Port=%d", tc.Bastion.User, tc.Bastion.Host, tc.Bastion.Port)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSshString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSshString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestParseGcloudSshString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantErr   bool
		check     func(t *testing.T, tc *config.TunnelConfig)
		errSubstr string
	}{
		{
			name:    "not a gcloud command returns nil nil",
			input:   "ssh -N -L 5432:localhost:5432",
			wantNil: true,
		},
		{
			name:    "missing tunnel-through-iap returns nil nil",
			input:   "gcloud compute ssh my-instance --zone=us-central1-a",
			wantNil: true,
		},
		{
			name:      "missing zone returns error",
			input:     "gcloud compute ssh my-instance --tunnel-through-iap --ssh-flag=\"-N -L 5432:localhost:5432\"",
			wantErr:   true,
			errSubstr: "zone",
		},
		{
			name:    "valid gcloud ssh IAP command",
			input:   "gcloud compute ssh my-instance --tunnel-through-iap --zone=us-west1-b --ssh-flag=\"-N -L 2345:10.26.0.3:5432\"",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.GcpIap == nil {
					t.Fatal("expected GcpIap to be set")
				}
				if tc.GcpIap.Zone != "us-west1-b" {
					t.Errorf("Zone = %q", tc.GcpIap.Zone)
				}
				if tc.Bastion.Host != "my-instance" {
					t.Errorf("Bastion.Host = %q", tc.Bastion.Host)
				}
				if tc.LocalPort != 2345 || tc.RemoteHost != "10.26.0.3" || tc.RemotePort != 5432 {
					t.Errorf("ports: local=%d remote=%s:%d", tc.LocalPort, tc.RemoteHost, tc.RemotePort)
				}
				if tc.Bastion.Port != 22 {
					t.Errorf("Bastion.Port = %d", tc.Bastion.Port)
				}
			},
		},
		{
			name:    "valid with project and user@instance",
			input:   "gcloud compute ssh alice@my-instance --tunnel-through-iap --zone=us-east1-a --project=my-proj --ssh-flag=\"-N -L 9000:db:5432\"",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.GcpIap.Zone != "us-east1-a" || tc.GcpIap.Project != "my-proj" {
					t.Errorf("GcpIap Zone=%q Project=%q", tc.GcpIap.Zone, tc.GcpIap.Project)
				}
				if tc.Bastion.User != "alice" || tc.Bastion.Host != "my-instance" {
					t.Errorf("Bastion User=%q Host=%q", tc.Bastion.User, tc.Bastion.Host)
				}
				if tc.LocalPort != 9000 || tc.RemoteHost != "db" || tc.RemotePort != 5432 {
					t.Errorf("ports: local=%d remote=%s:%d", tc.LocalPort, tc.RemoteHost, tc.RemotePort)
				}
			},
		},
		{
			name:    "valid with --configuration= extracts gcloud config",
			input:   "gcloud --configuration=myconfig compute ssh inst --tunnel-through-iap --zone=z1 --project=my-proj --ssh-flag=\"-N -L 5432:localhost:5432\"",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.GcpIap.GcloudConfiguration != "myconfig" {
					t.Errorf("GcpIap.GcloudConfiguration = %q, want myconfig", tc.GcpIap.GcloudConfiguration)
				}
			},
		},
		{
			name:      "no -L in ssh-flag returns error",
			input:     "gcloud compute ssh inst --tunnel-through-iap --zone=z1 --ssh-flag=\"-N\"",
			wantErr:   true,
			errSubstr: "port mapping",
		},
		{
			name:    "multiline paste normalizes",
			input:   "gcloud compute ssh inst --tunnel-through-iap\n  --zone=z1 --ssh-flag=\"-N -L 1:a:2\"",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.GcpIap.Zone != "z1" {
					t.Errorf("Zone = %q", tc.GcpIap.Zone)
				}
			},
		},
		{
			name:      "instance not found (only flags after ssh, no --ssh-flag)",
			input:     "gcloud compute ssh --tunnel-through-iap --zone=z1",
			wantErr:   true,
			errSubstr: "instance name not found",
		},
		{
			name:    "single-quoted ssh-flag",
			input:   "gcloud compute ssh my-instance --tunnel-through-iap --zone=us-west1-b --ssh-flag='-N -L 9999:db:5432'",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.LocalPort != 9999 || tc.RemoteHost != "db" || tc.RemotePort != 5432 {
					t.Errorf("ports: local=%d remote=%s:%d", tc.LocalPort, tc.RemoteHost, tc.RemotePort)
				}
			},
		},
		{
			name:    "4-part port mapping in gcloud",
			input:   "gcloud compute ssh inst --tunnel-through-iap --zone=z1 --ssh-flag=\"-N -L 0.0.0.0:3306:mysql:3306\"",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.BindAddress != "0.0.0.0" || tc.LocalPort != 3306 || tc.RemoteHost != "mysql" || tc.RemotePort != 3306 {
					t.Errorf("BindAddress=%s LocalPort=%d RemoteHost=%s RemotePort=%d", tc.BindAddress, tc.LocalPort, tc.RemoteHost, tc.RemotePort)
				}
			},
		},
		{
			name:      "gcloud 4-part invalid local port",
			input:     "gcloud compute ssh inst --tunnel-through-iap --zone=z1 --ssh-flag=\"-N -L bind:bad:host:80\"",
			wantErr:   true,
			errSubstr: "invalid local port",
		},
		{
			name:      "gcloud 4-part invalid remote port",
			input:     "gcloud compute ssh inst --tunnel-through-iap --zone=z1 --ssh-flag=\"-N -L 0.0.0.0:3306:mysql:nope\"",
			wantErr:   true,
			errSubstr: "invalid remote port",
		},
		{
			name:      "gcloud 3-part invalid remote port",
			input:     "gcloud compute ssh inst --tunnel-through-iap --zone=z1 --ssh-flag=\"-N -L 80:host:nope\"",
			wantErr:   true,
			errSubstr: "invalid remote port",
		},
		{
			name:      "gcloud invalid port mapping format",
			input:     "gcloud compute ssh inst --tunnel-through-iap --zone=z1 --ssh-flag=\"-N -L two:parts\"",
			wantErr:   true,
			errSubstr: "invalid port mapping format",
		},
		{
			name:      "gcloud remote host empty",
			input:     "gcloud compute ssh inst --tunnel-through-iap --zone=z1 --ssh-flag=\"-N -L 123::80\"",
			wantErr:   true,
			errSubstr: "remote host cannot be empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGcloudSshString(tt.input)
			if tt.wantNil {
				if got != nil || err != nil {
					t.Errorf("ParseGcloudSshString() expected (nil, nil), got (%v, %v)", got, err)
				}
				return
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGcloudSshString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errSubstr != "" && err != nil && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if tt.check != nil && got != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestParseGcloudStartIapTunnel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantErr   bool
		check     func(t *testing.T, tc *config.TunnelConfig)
		errSubstr string
	}{
		{
			name:    "not a start-iap-tunnel command returns nil nil",
			input:   "gcloud compute ssh inst --tunnel-through-iap --zone=z1",
			wantNil: true,
		},
		{
			name:      "missing zone returns error",
			input:     "gcloud compute start-iap-tunnel my-instance 5432",
			wantErr:   true,
			errSubstr: "zone",
		},
		{
			name:    "valid start-iap-tunnel",
			input:   "gcloud compute start-iap-tunnel rexly-envoy-prod-instance 5432 --zone=us-west1-b",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.GcpIap == nil {
					t.Fatal("expected GcpIap to be set")
				}
				if tc.GcpIap.Mode != "direct" {
					t.Errorf("Mode = %q", tc.GcpIap.Mode)
				}
				if tc.Bastion.Host != "rexly-envoy-prod-instance" || tc.RemotePort != 5432 {
					t.Errorf("Bastion.Host=%q RemotePort=%d", tc.Bastion.Host, tc.RemotePort)
				}
				if tc.GcpIap.Zone != "us-west1-b" {
					t.Errorf("Zone = %q", tc.GcpIap.Zone)
				}
				if tc.LocalPort != 5432 {
					t.Errorf("LocalPort = %d", tc.LocalPort)
				}
				if tc.BindAddress != "localhost" {
					t.Errorf("BindAddress = %q", tc.BindAddress)
				}
			},
		},
		{
			name:    "valid with project and local-host-port",
			input:   "gcloud compute start-iap-tunnel my-instance 22 --zone=us-central1-a --project=my-proj --local-host-port=localhost:2222",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.GcpIap.Project != "my-proj" {
					t.Errorf("Project = %q", tc.GcpIap.Project)
				}
				if tc.LocalPort != 2222 || tc.BindAddress != "localhost" {
					t.Errorf("LocalPort=%d BindAddress=%q", tc.LocalPort, tc.BindAddress)
				}
				if tc.RemotePort != 22 {
					t.Errorf("RemotePort = %d", tc.RemotePort)
				}
			},
		},
		{
			name:    "valid with --configuration= extracts gcloud config (direct mode)",
			input:   "gcloud --configuration=my-iap-config compute start-iap-tunnel my-instance 5432 --zone=us-west1-b --project=my-proj",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.GcpIap.GcloudConfiguration != "my-iap-config" {
					t.Errorf("GcpIap.GcloudConfiguration = %q, want my-iap-config", tc.GcpIap.GcloudConfiguration)
				}
			},
		},
		{
			name:      "malformed start-iap-tunnel (SSH-style flags) returns error",
			input:     "gcloud compute start-iap-tunnel rexly-envoy-prod-instance --tunnel-through-iap --zone=us-west1-b --ssh-flag=\"-N -L 2345:10.26.0.3:5432\"",
			wantErr:   true,
			errSubstr: "invalid instance port",
		},
		{
			name:    "valid with 4-part local-host-port",
			input:   "gcloud compute start-iap-tunnel inst 5432 --zone=z1 --local-host-port=0.0.0.0:5432",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.BindAddress != "0.0.0.0" || tc.LocalPort != 5432 {
					t.Errorf("BindAddress=%q LocalPort=%d", tc.BindAddress, tc.LocalPort)
				}
			},
		},
		{
			name:      "invalid instance port",
			input:     "gcloud compute start-iap-tunnel inst notanumber --zone=z1",
			wantErr:   true,
			errSubstr: "invalid instance port",
		},
		{
			name:      "instance name not found (only flags after start-iap-tunnel)",
			input:     "gcloud compute start-iap-tunnel --zone=z1",
			wantErr:   true,
			errSubstr: "instance name not found",
		},
		{
			name:      "instance port not found",
			input:     "gcloud compute start-iap-tunnel my-instance --zone=z1",
			wantErr:   true,
			errSubstr: "instance port not found",
		},
		{
			name:    "flags between start-iap-tunnel and instance",
			input:   "gcloud compute start-iap-tunnel --zone=z1 --foo my-inst 5432",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.Bastion.Host != "my-inst" || tc.LocalPort != 5432 || tc.RemotePort != 5432 {
					t.Errorf("Bastion.Host=%s LocalPort=%d RemotePort=%d", tc.Bastion.Host, tc.LocalPort, tc.RemotePort)
				}
			},
		},
		{
			name:    "local-host-port with only port number",
			input:   "gcloud compute start-iap-tunnel inst 22 --zone=z1 --local-host-port=9999",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.LocalPort != 9999 || tc.BindAddress != "localhost" {
					t.Errorf("LocalPort=%d BindAddress=%q", tc.LocalPort, tc.BindAddress)
				}
			},
		},
		{
			name:    "double spaces normalized",
			input:   "gcloud  compute  start-iap-tunnel  inst  5432  --zone=z1",
			wantNil: false,
			wantErr:  false,
			check: func(t *testing.T, tc *config.TunnelConfig) {
				if tc.Bastion.Host != "inst" || tc.LocalPort != 5432 {
					t.Errorf("Bastion.Host=%s LocalPort=%d", tc.Bastion.Host, tc.LocalPort)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGcloudStartIapTunnel(tt.input)
			if tt.wantNil {
				if got != nil || err != nil {
					t.Errorf("ParseGcloudStartIapTunnel() expected (nil, nil), got (%v, %v)", got, err)
				}
				return
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGcloudStartIapTunnel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errSubstr != "" && err != nil && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if tt.check != nil && got != nil {
				tt.check(t, got)
			}
		})
	}
}
