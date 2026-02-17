// Package mockwordpress provides a mock WordPress MCP server for evals.
// Config can be supplied via environment variables so the mock can validate
// against a real WordPress instance (e.g. staging) when credentials are set.
package mockwordpress

import (
	"os"
	"strings"
)

// Config holds WordPress connection settings (e.g. from env).
type Config struct {
	WebsiteURL string // WordPress site URL (e.g. https://example.com)
	Username   string
	AppPassword string
}

// FromEnv reads config from environment variables.
// OBOT_EVAL_WP_URL, OBOT_EVAL_WP_USERNAME, OBOT_EVAL_WP_APP_PASSWORD.
func FromEnv() Config {
	url := os.Getenv("OBOT_EVAL_WP_URL")
	url = strings.TrimSuffix(url, "/")
	return Config{
		WebsiteURL: url,
		Username:   os.Getenv("OBOT_EVAL_WP_USERNAME"),
		AppPassword: os.Getenv("OBOT_EVAL_WP_APP_PASSWORD"),
	}
}

// Set returns true if at least URL and username are set (enough to attempt connection).
func (c Config) Set() bool {
	return c.WebsiteURL != "" && c.Username != ""
}
