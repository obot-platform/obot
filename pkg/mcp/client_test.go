package mcp

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitIntoMap(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected map[string]string
	}{
		{
			name:     "empty list",
			input:    []string{},
			expected: map[string]string{},
		},
		{
			name:     "nil list",
			input:    nil,
			expected: map[string]string{},
		},
		{
			name:  "single key-value pair",
			input: []string{"KEY=value"},
			expected: map[string]string{
				"KEY": "value",
			},
		},
		{
			name:  "multiple key-value pairs",
			input: []string{"KEY1=value1", "KEY2=value2", "KEY3=value3"},
			expected: map[string]string{
				"KEY1": "value1",
				"KEY2": "value2",
				"KEY3": "value3",
			},
		},
		{
			name:  "value with equals sign",
			input: []string{"KEY=value=with=equals"},
			expected: map[string]string{
				"KEY": "value=with=equals",
			},
		},
		{
			name:  "empty value",
			input: []string{"KEY="},
			expected: map[string]string{
				"KEY": "",
			},
		},
		{
			name:     "no equals sign - ignored",
			input:    []string{"INVALID"},
			expected: map[string]string{},
		},
		{
			name:  "mixed valid and invalid entries",
			input: []string{"KEY1=value1", "INVALID", "KEY2=value2"},
			expected: map[string]string{
				"KEY1": "value1",
				"KEY2": "value2",
			},
		},
		{
			name:  "spaces in values",
			input: []string{"KEY=value with spaces"},
			expected: map[string]string{
				"KEY": "value with spaces",
			},
		},
		{
			name:  "special characters in values",
			input: []string{"KEY=!@#$%^&*()"},
			expected: map[string]string{
				"KEY": "!@#$%^&*()",
			},
		},
		{
			name:  "authorization header format",
			input: []string{"Authorization=Bearer token123"},
			expected: map[string]string{
				"Authorization": "Bearer token123",
			},
		},
		{
			name:  "duplicate keys - last one wins",
			input: []string{"KEY=first", "KEY=second"},
			expected: map[string]string{
				"KEY": "second",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitIntoMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClientHasValidToken(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		setupClient func() *Client
		expected    bool
	}{
		{
			name: "nil jwt token",
			setupClient: func() *Client {
				return &Client{
					jwt: nil,
				}
			},
			expected: false,
		},
		{
			name: "valid token - expires in 10 minutes",
			setupClient: func() *Client {
				claims := jwt.MapClaims{
					"exp": float64(now.Add(10 * time.Minute).Unix()),
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				return &Client{
					jwt: token,
				}
			},
			expected: true,
		},
		{
			name: "valid token - expires in exactly 5 minutes",
			setupClient: func() *Client {
				claims := jwt.MapClaims{
					"exp": float64(now.Add(5 * time.Minute).Unix()),
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				return &Client{
					jwt: token,
				}
			},
			expected: false, // Not valid because we require 5 minutes buffer
		},
		{
			name: "invalid token - expires in 4 minutes",
			setupClient: func() *Client {
				claims := jwt.MapClaims{
					"exp": float64(now.Add(4 * time.Minute).Unix()),
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				return &Client{
					jwt: token,
				}
			},
			expected: false,
		},
		{
			name: "expired token",
			setupClient: func() *Client {
				claims := jwt.MapClaims{
					"exp": float64(now.Add(-1 * time.Minute).Unix()),
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				return &Client{
					jwt: token,
				}
			},
			expected: false,
		},
		{
			name: "token with no expiration - treated as valid",
			setupClient: func() *Client {
				claims := jwt.MapClaims{}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				return &Client{
					jwt: token,
				}
			},
			expected: true,
		},
		{
			name: "token with nil expiration - error on parsing",
			setupClient: func() *Client {
				claims := jwt.MapClaims{
					"exp": nil,
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				return &Client{
					jwt: token,
				}
			},
			expected: false, // GetExpirationTime returns error for nil exp
		},
		{
			name: "token expiring exactly at buffer threshold (5 min + 1 second)",
			setupClient: func() *Client {
				claims := jwt.MapClaims{
					"exp": float64(now.Add(5*time.Minute + time.Second).Unix()),
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				return &Client{
					jwt: token,
				}
			},
			expected: true,
		},
		{
			name: "token with invalid exp type - treated as error",
			setupClient: func() *Client {
				claims := jwt.MapClaims{
					"exp": "invalid",
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				return &Client{
					jwt: token,
				}
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			result := client.hasValidToken()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandEnvVars(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		credEnv     map[string]string
		fileEnvVars map[string]struct{}
		expected    string
	}{
		{
			name:        "no variables to expand",
			text:        "simple text",
			credEnv:     map[string]string{"VAR": "value"},
			fileEnvVars: map[string]struct{}{},
			expected:    "simple text",
		},
		{
			name:        "single variable expansion",
			text:        "Hello ${NAME}",
			credEnv:     map[string]string{"NAME": "World"},
			fileEnvVars: map[string]struct{}{},
			expected:    "Hello World",
		},
		{
			name:        "multiple variables",
			text:        "${GREETING} ${NAME}!",
			credEnv:     map[string]string{"GREETING": "Hello", "NAME": "Alice"},
			fileEnvVars: map[string]struct{}{},
			expected:    "Hello Alice!",
		},
		{
			name:        "variable not in credEnv - left unchanged",
			text:        "Hello ${UNKNOWN}",
			credEnv:     map[string]string{"NAME": "World"},
			fileEnvVars: map[string]struct{}{},
			expected:    "Hello ${UNKNOWN}",
		},
		{
			name:        "nil credEnv - no expansion",
			text:        "Hello ${NAME}",
			credEnv:     nil,
			fileEnvVars: map[string]struct{}{},
			expected:    "Hello ${NAME}",
		},
		{
			name:        "empty credEnv - no expansion",
			text:        "Hello ${NAME}",
			credEnv:     map[string]string{},
			fileEnvVars: map[string]struct{}{},
			expected:    "Hello ${NAME}",
		},
		{
			name:        "file env var - not expanded",
			text:        "Config: ${FILE_VAR}",
			credEnv:     map[string]string{"FILE_VAR": "should-not-expand"},
			fileEnvVars: map[string]struct{}{"FILE_VAR": {}},
			expected:    "Config: ${FILE_VAR}",
		},
		{
			name:        "mixed file and non-file vars",
			text:        "${NORMAL} and ${FILE_VAR}",
			credEnv:     map[string]string{"NORMAL": "expanded", "FILE_VAR": "not-expanded"},
			fileEnvVars: map[string]struct{}{"FILE_VAR": {}},
			expected:    "expanded and ${FILE_VAR}",
		},
		{
			name:        "URL with variable",
			text:        "https://${HOST}/api/${VERSION}",
			credEnv:     map[string]string{"HOST": "example.com", "VERSION": "v1"},
			fileEnvVars: map[string]struct{}{},
			expected:    "https://example.com/api/v1",
		},
		{
			name:        "empty variable value",
			text:        "Value: ${EMPTY}",
			credEnv:     map[string]string{"EMPTY": ""},
			fileEnvVars: map[string]struct{}{},
			expected:    "Value: ",
		},
		{
			name:        "variable with special characters in value",
			text:        "Token: ${TOKEN}",
			credEnv:     map[string]string{"TOKEN": "abc!@#$%^&*()"},
			fileEnvVars: map[string]struct{}{},
			expected:    "Token: abc!@#$%^&*()",
		},
		{
			name:        "adjacent variables",
			text:        "${A}${B}${C}",
			credEnv:     map[string]string{"A": "1", "B": "2", "C": "3"},
			fileEnvVars: map[string]struct{}{},
			expected:    "123",
		},
		{
			name:        "malformed variable (no closing brace)",
			text:        "${INCOMPLETE",
			credEnv:     map[string]string{"INCOMPLETE": "value"},
			fileEnvVars: map[string]struct{}{},
			expected:    "${INCOMPLETE",
		},
		{
			name:        "nested braces - regex matches first closing brace",
			text:        "${VAR{inner}}",
			credEnv:     map[string]string{"VAR{inner": "value"},
			fileEnvVars: map[string]struct{}{},
			expected:    "value}", // Regex matches ${VAR{inner} as the var name
		},
		{
			name:        "dollar sign without braces",
			text:        "Cost: $100",
			credEnv:     map[string]string{},
			fileEnvVars: map[string]struct{}{},
			expected:    "Cost: $100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandEnvVars(tt.text, tt.credEnv, tt.fileEnvVars)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyPrefix(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		prefix   string
		expected string
	}{
		{
			name:     "empty value - no prefix added",
			value:    "",
			prefix:   "Bearer ",
			expected: "",
		},
		{
			name:     "empty prefix - value unchanged",
			value:    "token123",
			prefix:   "",
			expected: "token123",
		},
		{
			name:     "value already has prefix - not duplicated",
			value:    "Bearer token123",
			prefix:   "Bearer ",
			expected: "Bearer token123",
		},
		{
			name:     "value needs prefix",
			value:    "token123",
			prefix:   "Bearer ",
			expected: "Bearer token123",
		},
		{
			name:     "OpenAI style sk- prefix",
			value:    "proj-abc123",
			prefix:   "sk-",
			expected: "sk-proj-abc123",
		},
		{
			name:     "OpenAI key already has sk- prefix",
			value:    "sk-proj-abc123",
			prefix:   "sk-",
			expected: "sk-proj-abc123",
		},
		{
			name:     "Token prefix",
			value:    "my-token",
			prefix:   "Token ",
			expected: "Token my-token",
		},
		{
			name:     "value starts with prefix substring but not full prefix",
			value:    "Bearertoken",
			prefix:   "Bearer ",
			expected: "Bearer Bearertoken",
		},
		{
			name:     "case sensitive - different case not treated as prefix",
			value:    "bearer token",
			prefix:   "Bearer ",
			expected: "Bearer bearer token",
		},
		{
			name:     "both empty",
			value:    "",
			prefix:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyPrefix(tt.value, tt.prefix)
			require.Equal(t, tt.expected, result)
		})
	}
}
