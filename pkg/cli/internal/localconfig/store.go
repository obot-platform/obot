package localconfig

// Config is the setup-managed CLI configuration written under the
// user's XDG config directory.
type Config struct {
	DefaultURL string `json:"defaultURL,omitempty"`
}

// Store is the local configuration storage boundary.
type Store interface {
	Load() (Config, error)
	Save(Config) error
}
