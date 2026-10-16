package client

import (
	"errors"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// A guardrail's settings and secrets are saved together: if either write fails, neither is.
func TestGuardrailAndSecretsSaveTogether(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	if err := c.db.WithContext(ctx).AutoMigrate(&types.Guardrail{}); err != nil {
		t.Fatal(err)
	}
	guardrail := &types.Guardrail{Name: "p", Type: "lakera", Config: []byte(`{"baseURL":"https://api.lakera.ai"}`)}
	if err := c.CreateGuardrail(ctx, guardrail, map[string]string{"apiKey": "first"}); err != nil {
		t.Fatal(err)
	}

	guardrail.Config = []byte(`{"baseURL":"https://other.example"}`)
	if err := c.UpdateGuardrail(ctx, guardrail, map[string]string{"apiKey": "second"}); err != nil {
		t.Fatal(err)
	}
	if secrets, err := c.GuardrailSecrets(ctx, guardrail.ID); err != nil || secrets["apiKey"] != "second" {
		t.Fatalf("secrets = %v, %v; want the new key", secrets, err)
	}

	// A failed guardrail write leaves the stored secrets as they were.
	failed := c.saveGuardrail(ctx, guardrail.ID, map[string]string{"apiKey": "third"}, func(*gorm.DB) error {
		return errors.New("write failed")
	})
	if failed == nil {
		t.Fatal("expected the failed write to be reported")
	}
	if secrets, err := c.GuardrailSecrets(ctx, guardrail.ID); err != nil || secrets["apiKey"] != "second" {
		t.Fatalf("secrets = %v, %v; want them unchanged after a failed write", secrets, err)
	}

	// No secrets removes the stored ones.
	if err := c.UpdateGuardrail(ctx, guardrail, nil); err != nil {
		t.Fatal(err)
	}
	if secrets, err := c.GuardrailSecrets(ctx, guardrail.ID); err != nil || len(secrets) != 0 {
		t.Fatalf("secrets = %v, %v; want none", secrets, err)
	}
}
