package encryption

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

func TestInitCustomProviderBuildsAESConfigFromManagedKey(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	config, err := Init(t.Context(), Options{EncryptionProvider: "custom", EncryptionKey: key})
	if err != nil {
		t.Fatalf("initialize custom encryption from managed key: %v", err)
	}

	for _, resource := range []schema.GroupResource{
		{Group: "obot.obot.ai", Resource: "credentials"},
		{Group: "obot.obot.ai", Resource: "mcpoauthpendingstates"},
	} {
		transformer := config.Transformers[resource]
		if transformer == nil {
			t.Fatalf("missing transformer for %s", resource)
		}
		dataContext := value.DefaultContext(resource.String())
		ciphertext, err := transformer.TransformToStorage(context.Background(), []byte("candidate-secret"), dataContext)
		if err != nil {
			t.Fatalf("encrypt %s: %v", resource, err)
		}
		if string(ciphertext) == "candidate-secret" {
			t.Fatalf("%s transformer stored plaintext", resource)
		}
		plaintext, _, err := transformer.TransformFromStorage(context.Background(), ciphertext, dataContext)
		if err != nil {
			t.Fatalf("decrypt %s: %v", resource, err)
		}
		if string(plaintext) != "candidate-secret" {
			t.Fatalf("%s round trip = %q", resource, plaintext)
		}
	}
}

func TestCustomProviderRejectsMissingOrInvalidManagedKey(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  string
	}{
		{name: "missing"},
		{name: "invalid", key: "not-base64"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Init(t.Context(), Options{EncryptionProvider: "custom", EncryptionKey: tt.key}); err == nil {
				t.Fatal("custom encryption accepted an unusable managed key")
			}
		})
	}
}

func TestManagedAESConfigDecryptsRetiredKeyAndWritesWithActiveKey(t *testing.T) {
	oldKey := base64.StdEncoding.EncodeToString([]byte("old-key-0123456789abcdef01234567"))
	newKey := base64.StdEncoding.EncodeToString([]byte("new-key-0123456789abcdef01234567"))
	resource := schema.GroupResource{Group: "obot.obot.ai", Resource: "credentials"}
	dataContext := value.DefaultContext(resource.String())

	oldConfig, err := Init(t.Context(), Options{
		EncryptionProvider: "custom",
		EncryptionKey:      oldKey,
		EncryptionKeyID:    "k1",
	})
	if err != nil {
		t.Fatalf("initialize old key: %v", err)
	}
	oldCiphertext, err := oldConfig.Transformers[resource].TransformToStorage(t.Context(), []byte("secret"), dataContext)
	if err != nil {
		t.Fatalf("encrypt with old key: %v", err)
	}

	rotatedConfig, err := Init(t.Context(), Options{
		EncryptionProvider:    "custom",
		EncryptionKey:         newKey,
		EncryptionKeyID:       "k2",
		EncryptionRetiredKeys: `{"k1":"` + oldKey + `"}`,
	})
	if err != nil {
		t.Fatalf("initialize rotated keyring: %v", err)
	}
	plaintext, stale, err := rotatedConfig.Transformers[resource].TransformFromStorage(t.Context(), oldCiphertext, dataContext)
	if err != nil {
		t.Fatalf("decrypt with retired key: %v", err)
	}
	if string(plaintext) != "secret" || !stale {
		t.Fatalf("retired-key decrypt = %q stale=%v, want secret/true", plaintext, stale)
	}

	newCiphertext, err := rotatedConfig.Transformers[resource].TransformToStorage(t.Context(), []byte("secret"), dataContext)
	if err != nil {
		t.Fatalf("encrypt with active key: %v", err)
	}
	if !bytes.Contains(newCiphertext, []byte(":k2:")) {
		t.Fatalf("new ciphertext %q does not use active key ID k2", newCiphertext)
	}
}
