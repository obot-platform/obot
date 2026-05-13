package localconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
)

const configRelPath = "obot/config.json"

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

type xdgStore struct{}

// NormalizeAppURL returns the canonical Obot app URL used for local
// config and keyring lookup.
func NormalizeAppURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("app URL is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid app URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("app URL must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("app URL must include a host")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("app URL must not include user info")
	}
	if parsed.RawQuery != "" {
		return "", fmt.Errorf("app URL must not include a query string")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("app URL must not include a fragment")
	}

	normalized := strings.TrimRight(parsed.String(), "/")
	normalized = strings.TrimSuffix(normalized, "/api")
	return normalized, nil
}

// APIBaseURL derives the Obot API base URL from a normalized app URL.
func APIBaseURL(appURL string) string {
	return strings.TrimRight(appURL, "/") + "/api"
}

// Load reads the setup-managed CLI config from XDG config storage.
func Load() (Config, error) {
	return xdgStore{}.Load()
}

// Save writes the setup-managed CLI config to XDG config storage.
func Save(cfg Config) error {
	return xdgStore{}.Save(cfg)
}

// ActiveAppURL resolves the active app URL using an explicit value
// first, then the stored default URL.
func ActiveAppURL(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return NormalizeAppURL(explicit)
	}

	cfg, err := Load()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cfg.DefaultURL) == "" {
		return "", errors.New("no Obot URL configured")
	}
	return NormalizeAppURL(cfg.DefaultURL)
}

func (xdgStore) Load() (Config, error) {
	path := filepath.Join(xdg.ConfigHome, configRelPath)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if cfg.DefaultURL != "" {
		defaultURL, err := NormalizeAppURL(cfg.DefaultURL)
		if err != nil {
			return Config{}, fmt.Errorf("reading %s: %w", path, err)
		}
		cfg.DefaultURL = defaultURL
	}
	return cfg, nil
}

func (xdgStore) Save(cfg Config) error {
	if cfg.DefaultURL != "" {
		defaultURL, err := NormalizeAppURL(cfg.DefaultURL)
		if err != nil {
			return err
		}
		cfg.DefaultURL = defaultURL
	}

	path, err := xdg.ConfigFile(configRelPath)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
