package tunnel

import (
	"strings"
	"testing"
)

func TestTunnelPeerConfigValidate(t *testing.T) {
	complete := PeerConfig{
		ID:               "pod-a-uid",
		Token:            "peer-token",
		ServiceName:      "obot",
		ServiceNamespace: "obot-system",
	}

	tests := []struct {
		name    string
		config  PeerConfig
		wantErr string
	}{
		{
			name: "disabled",
		},
		{
			name:   "whitespace is disabled",
			config: PeerConfig{ID: "  "},
		},
		{
			name:   "complete",
			config: complete,
		},
		{
			name:    "only ID",
			config:  PeerConfig{ID: "pod-a-uid"},
			wantErr: "missing Token, ServiceName, ServiceNamespace",
		},
		{
			name: "missing token",
			config: PeerConfig{
				ID:               complete.ID,
				ServiceName:      complete.ServiceName,
				ServiceNamespace: complete.ServiceNamespace,
			},
			wantErr: "missing Token",
		},
		{
			name:    "only service name",
			config:  PeerConfig{ServiceName: "obot"},
			wantErr: "missing ID, Token, ServiceNamespace",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				wantEnabled := test.name == "complete"
				if got := test.config.Enabled(); got != wantEnabled {
					t.Fatalf("Enabled() = %v, want %v", got, wantEnabled)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantErr)
			}
			if test.config.Enabled() {
				t.Fatal("invalid partial config is enabled")
			}
		})
	}
}
