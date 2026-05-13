package credentials

import (
	"errors"

	keyringlib "github.com/zalando/go-keyring"
)

// keyring is the subset of github.com/zalando/go-keyring used by the
// CLI. Tests can provide an in-memory implementation.
type keyring interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
	Delete(service, user string) error
}

type keyringFuncs struct{}

func (keyringFuncs) Get(service, user string) (string, error) {
	return keyringlib.Get(service, user)
}

func (keyringFuncs) Set(service, user, secret string) error {
	return keyringlib.Set(service, user, secret)
}

func (keyringFuncs) Delete(service, user string) error {
	return keyringlib.Delete(service, user)
}

// KeyringStore stores one bearer token per normalized Obot app URL.
type KeyringStore struct {
	service string
	keyring keyring
}

// NewKeyringStore returns a Store backed by the host OS keyring.
func NewKeyringStore() *KeyringStore {
	return newKeyringStoreWith(DefaultService, keyringFuncs{})
}

func newKeyringStoreWith(service string, keyring keyring) *KeyringStore {
	if service == "" {
		service = DefaultService
	}
	return &KeyringStore{
		service: service,
		keyring: keyring,
	}
}

func (s *KeyringStore) Get(appURL string) (string, error) {
	token, err := s.keyring.Get(s.service, appURL)
	if errors.Is(err, keyringlib.ErrNotFound) {
		return "", ErrNotFound
	}
	return token, err
}

func (s *KeyringStore) Set(appURL, token string) error {
	return s.keyring.Set(s.service, appURL, token)
}

func (s *KeyringStore) Delete(appURL string) error {
	err := s.keyring.Delete(s.service, appURL)
	if errors.Is(err, keyringlib.ErrNotFound) {
		return nil
	}
	return err
}
