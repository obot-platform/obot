package obothelmvalues

import (
	"strings"
	"testing"
)

func TestMaskValuesYAMLMasksConfig(t *testing.T) {
	maskedYAML, err := MaskValuesYAML(`
config:
  existingSecret: custom-secret
  OBOT_SERVER_ENABLE_AUTHENTICATION: true
  OPENAI_API_KEY: sk-test
service:
  type: ClusterIP
  port: 80
  annotations:
    example.com/setting: enabled
`)
	if err != nil {
		t.Fatalf("MaskValuesYAML() error = %v", err)
	}
	if strings.Contains(maskedYAML, "custom-secret") || strings.Contains(maskedYAML, "sk-test") {
		t.Fatalf("masked yaml should not contain secrets: %q", maskedYAML)
	}
	if !strings.Contains(maskedYAML, "OPENAI_API_KEY: "+MaskedValue) && !strings.Contains(maskedYAML, "OPENAI_API_KEY: '"+MaskedValue+"'") {
		t.Fatalf("masked yaml = %q, want masked config keys", maskedYAML)
	}
	if !strings.Contains(maskedYAML, MaskedValue) {
		t.Fatalf("masked yaml = %q, want masked annotation values", maskedYAML)
	}
}

func TestMaskValuesConfigMasksAllKeys(t *testing.T) {
	masked := MaskValues(map[string]any{
		"config": map[string]any{
			"existingSecret": "custom-secret",
			"OPENAI_API_KEY": "sk-test",
		},
	})

	config, ok := masked["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %#v, want map", masked["config"])
	}
	for key, value := range config {
		if value != MaskedValue {
			t.Fatalf("config[%q] = %v, want masked value", key, value)
		}
	}
}

func TestMaskValuesConfigOmitsEmptyStrings(t *testing.T) {
	masked := MaskValues(map[string]any{
		"config": map[string]any{
			"OPENAI_API_KEY":                    "sk-test",
			"OBOT_SERVER_AUDIT_LOGS_STORE_S3ENDPOINT": "",
			"OBOT_SERVER_ENABLE_AUTHENTICATION": false,
		},
	})

	config, ok := masked["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %#v, want map", masked["config"])
	}
	if _, ok := config["OBOT_SERVER_AUDIT_LOGS_STORE_S3ENDPOINT"]; ok {
		t.Fatal("expected empty config values to be omitted")
	}
	if config["OPENAI_API_KEY"] != MaskedValue {
		t.Fatalf("OPENAI_API_KEY = %v, want masked value", config["OPENAI_API_KEY"])
	}
	if config["OBOT_SERVER_ENABLE_AUTHENTICATION"] != MaskedValue {
		t.Fatalf("OBOT_SERVER_ENABLE_AUTHENTICATION = %v, want masked false boolean", config["OBOT_SERVER_ENABLE_AUTHENTICATION"])
	}
}

func TestMaskStringMapValues(t *testing.T) {
	masked := maskStringMapValues(map[string]string{
		"example.com/setting": "enabled",
		"auth-token":          "abc123",
		"empty-value":         "",
	})
	if masked["example.com/setting"] != MaskedValue {
		t.Fatalf("non-empty annotation value should be masked, got %q", masked["example.com/setting"])
	}
	if masked["auth-token"] != MaskedValue {
		t.Fatalf("sensitive key should be masked, got %q", masked["auth-token"])
	}
	if masked["empty-value"] != "" {
		t.Fatalf("empty value should remain empty, got %q", masked["empty-value"])
	}
}
