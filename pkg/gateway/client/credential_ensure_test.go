package client

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestEnsureCredentialKeepsTheFirst(t *testing.T) {
	c := newTestClient(t)
	first, err := c.EnsureCredential(t.Context(), types.Credential{Context: "llm-gateway", Name: "encryption-key", Secrets: map[string]string{"key": "a"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.EnsureCredential(t.Context(), types.Credential{Context: "llm-gateway", Name: "encryption-key", Secrets: map[string]string{"key": "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Secrets["key"] != "a" || second.Secrets["key"] != "a" {
		t.Fatalf("expected the first key to be kept, got %q and %q", first.Secrets["key"], second.Secrets["key"])
	}
}
