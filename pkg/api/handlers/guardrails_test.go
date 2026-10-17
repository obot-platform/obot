package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

func TestGuardrailPoliciesAreLLMOnly(t *testing.T) {
	policy := gatewaytypes.GuardrailPolicy{Name: "r", Target: gatewaytypes.GuardrailTargetMCP}
	err := validateGuardrailPolicy(api.Context{}, &policy)
	if err == nil || !strings.Contains(err.Error(), "guardrail filter") {
		t.Fatalf("an MCP policy must be rejected, got %v", err)
	}
}

func TestCheckDestinationChange(t *testing.T) {
	existing := &gatewaytypes.Guardrail{Type: "presidio", Config: json.RawMessage(`{"analyzerURL":"https://presidio.internal"}`)}
	stored := map[string]string{"apiKey": "secret"}

	tests := []struct {
		name    string
		config  string
		stored  map[string]string
		wantErr bool
	}{
		{
			name:    "redirecting the stored key is refused",
			config:  `{"analyzerURL":"https://attacker.example"}`,
			stored:  stored,
			wantErr: true,
		},
		{
			name:   "a new key may go to a new endpoint",
			config: `{"analyzerURL":"https://attacker.example","apiKey":"new"}`,
			stored: stored,
		},
		{
			name:   "other settings may change while keeping the key",
			config: `{"analyzerURL":"https://presidio.internal","scoreThreshold":0.9}`,
			stored: stored,
		},
		{
			name:    "a key that isn't a string doesn't count as re-entered",
			config:  `{"analyzerURL":"https://attacker.example","apiKey":1}`,
			stored:  stored,
			wantErr: true,
		},
		{
			name:    "a blank key doesn't count as re-entered",
			config:  `{"analyzerURL":"https://attacker.example","apiKey":"  "}`,
			stored:  stored,
			wantErr: true,
		},
		{
			name:   "nothing stored, nothing to protect",
			config: `{"analyzerURL":"https://elsewhere.example"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated := &gatewaytypes.Guardrail{Type: "presidio", Config: json.RawMessage(tt.config)}
			err := checkDestinationChange(existing, updated, tt.stored)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// A sensitive setting that isn't a string is refused, not read as omitted: reading it as
// omitted would keep the stored secret while the destination check saw a new one.
func TestSplitGuardrailSecretsRefusesNonStringSecrets(t *testing.T) {
	stored := map[string]string{"apiKey": "secret"}
	for _, value := range []string{`1`, `true`, `[]`, `{}`} {
		guardrail := &gatewaytypes.Guardrail{Name: "p", Type: "presidio", Config: json.RawMessage(`{"analyzerURL":"https://presidio.internal","apiKey":` + value + `}`)}
		if _, err := splitGuardrailSecrets(guardrail, stored); err == nil {
			t.Errorf("apiKey %s: want an error", value)
		}
	}

	for _, tt := range []struct {
		name   string
		config string
		want   string
	}{
		{
			name:   "omitted keeps the stored key",
			config: `{"analyzerURL":"https://presidio.internal"}`,
			want:   "secret",
		},
		{
			name:   "null keeps the stored key",
			config: `{"analyzerURL":"https://presidio.internal","apiKey":null}`,
			want:   "secret",
		},
		{
			name:   "a new key replaces it",
			config: `{"analyzerURL":"https://presidio.internal","apiKey":" new "}`,
			want:   "new",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			guardrail := &gatewaytypes.Guardrail{Name: "p", Type: "presidio", Config: json.RawMessage(tt.config)}
			secrets, err := splitGuardrailSecrets(guardrail, stored)
			if err != nil {
				t.Fatal(err)
			}
			if secrets["apiKey"] != tt.want {
				t.Fatalf("apiKey = %q, want %q", secrets["apiKey"], tt.want)
			}
			if strings.Contains(string(guardrail.Config), "apiKey") {
				t.Fatalf("the key stayed in the config: %s", guardrail.Config)
			}
		})
	}
}
