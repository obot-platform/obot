package credentials

import (
	"errors"
	"testing"

	keyringlib "github.com/zalando/go-keyring"
)

type fakeKeyring struct {
	secrets map[[2]string]string
	err     error
}

func newFakeKeyring() *fakeKeyring {
	return &fakeKeyring{secrets: map[[2]string]string{}}
}

func (f *fakeKeyring) Get(service, user string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	secret, ok := f.secrets[[2]string{service, user}]
	if !ok {
		return "", keyringlib.ErrNotFound
	}
	return secret, nil
}

func (f *fakeKeyring) Set(service, user, secret string) error {
	if f.err != nil {
		return f.err
	}
	f.secrets[[2]string{service, user}] = secret
	return nil
}

func (f *fakeKeyring) Delete(service, user string) error {
	if f.err != nil {
		return f.err
	}
	key := [2]string{service, user}
	if _, ok := f.secrets[key]; !ok {
		return keyringlib.ErrNotFound
	}
	delete(f.secrets, key)
	return nil
}

func TestKeyringStoreScopesTokensByAppURL(t *testing.T) {
	kr := newFakeKeyring()
	store := newKeyringStoreWith("obot-test", kr)

	if err := store.Set("https://obot.example.com", "token-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("https://other.example.com", "token-b"); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get("https://obot.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "token-a" {
		t.Fatalf("expected token-a, got %q", got)
	}
}

func TestKeyringStoreMapsNotFound(t *testing.T) {
	store := newKeyringStoreWith("obot-test", newFakeKeyring())

	_, err := store.Get("https://obot.example.com")
	if !IsNotFound(err) {
		t.Fatalf("expected ErrNotFound from Get, got %v", err)
	}

	err = store.Delete("https://obot.example.com")
	if err != nil {
		t.Fatalf("expected nil from Delete, got %v", err)
	}
}

func TestKeyringStorePreservesKeyringErrors(t *testing.T) {
	keyringErr := errors.New("keyring unavailable")
	kr := newFakeKeyring()
	kr.err = keyringErr
	store := newKeyringStoreWith("obot-test", kr)

	if _, err := store.Get("https://obot.example.com"); !errors.Is(err, keyringErr) {
		t.Fatalf("expected keyring error from Get, got %v", err)
	}
	if err := store.Set("https://obot.example.com", "token"); !errors.Is(err, keyringErr) {
		t.Fatalf("expected keyring error from Set, got %v", err)
	}
	if err := store.Delete("https://obot.example.com"); !errors.Is(err, keyringErr) {
		t.Fatalf("expected keyring error from Delete, got %v", err)
	}
}
