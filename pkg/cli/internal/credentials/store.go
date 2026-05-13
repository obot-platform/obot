package credentials

import "errors"

const (
	// DefaultService is the OS keyring service name used for Obot CLI
	// credentials.
	DefaultService = "obot"
)

// ErrNotFound is returned when no credential exists for the requested
// app URL.
var ErrNotFound = errors.New("credential not found")

// Store is the credential storage boundary for Obot app URL scoped
// bearer tokens.
type Store interface {
	Get(appURL string) (string, error)
	Set(appURL, token string) error
	// Delete removes the credential for appURL. Missing credentials are
	// not an error.
	Delete(appURL string) error
}

// IsNotFound reports whether err means a credential is absent.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
