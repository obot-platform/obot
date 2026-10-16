package llmengine

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type memoryKeyStore struct {
	lock sync.Mutex
	key  string
}

func (m *memoryKeyStore) EnsureEncryptionKey(key string) (string, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.key == "" {
		m.key = key
	}
	return m.key, nil
}

func TestSharedEncryptionKey(t *testing.T) {
	store := &memoryKeyStore{}

	// The first replica already has a key from before keys were shared; it is kept, so the
	// provider keys it encrypted stay readable.
	first := t.TempDir()
	if err := os.WriteFile(filepath.Join(first, "encryption.key"), []byte("existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	key1, err := resolveEncryptionKey(Options{DataDir: first, SharedKeys: store})
	if err != nil {
		t.Fatal(err)
	}
	// A new replica with an empty data dir gets the same key.
	key2, err := resolveEncryptionKey(Options{DataDir: t.TempDir(), SharedKeys: store})
	if err != nil {
		t.Fatal(err)
	}
	if key1 != "existing" || key2 != "existing" {
		t.Fatalf("replicas must share the key, got %q and %q", key1, key2)
	}

	// An explicit key always wins.
	if key, _ := resolveEncryptionKey(Options{DataDir: t.TempDir(), SharedKeys: store, EncryptionKey: "explicit"}); key != "explicit" {
		t.Fatalf("expected the explicit key, got %q", key)
	}
}
