package types

import (
	"testing"

	"github.com/gptscript-ai/go-gptscript"
)

func TestToFields(t *testing.T) {
	tests := []struct {
		name     string
		input    gptscript.Fields
		expected Fields
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: Fields{},
		},
		{
			name:     "empty slice",
			input:    gptscript.Fields{},
			expected: Fields{},
		},
		{
			name: "single field without sensitive",
			input: gptscript.Fields{
				{Name: "username", Description: "User name"},
			},
			expected: Fields{
				{Name: "username", Description: "User name", Sensitive: nil, Options: nil},
			},
		},
		{
			name: "single field with sensitive true",
			input: gptscript.Fields{
				{Name: "password", Description: "Password", Sensitive: boolPtr(true)},
			},
			expected: Fields{
				{Name: "password", Description: "Password", Sensitive: boolPtr(true), Options: nil},
			},
		},
		{
			name: "single field with sensitive false",
			input: gptscript.Fields{
				{Name: "email", Description: "Email address", Sensitive: boolPtr(false)},
			},
			expected: Fields{
				{Name: "email", Description: "Email address", Sensitive: boolPtr(false), Options: nil},
			},
		},
		{
			name: "field with options",
			input: gptscript.Fields{
				{Name: "role", Description: "User role", Options: []string{"admin", "user", "guest"}},
			},
			expected: Fields{
				{Name: "role", Description: "User role", Sensitive: nil, Options: []string{"admin", "user", "guest"}},
			},
		},
		{
			name: "multiple fields with mixed properties",
			input: gptscript.Fields{
				{Name: "username", Description: "User name"},
				{Name: "password", Description: "Password", Sensitive: boolPtr(true)},
				{Name: "role", Description: "User role", Options: []string{"admin", "user"}},
			},
			expected: Fields{
				{Name: "username", Description: "User name", Sensitive: nil, Options: nil},
				{Name: "password", Description: "Password", Sensitive: boolPtr(true), Options: nil},
				{Name: "role", Description: "User role", Sensitive: nil, Options: []string{"admin", "user"}},
			},
		},
		{
			name: "field with all properties",
			input: gptscript.Fields{
				{
					Name:        "apiKey",
					Description: "API Key",
					Sensitive:   boolPtr(true),
					Options:     []string{"key1", "key2"},
				},
			},
			expected: Fields{
				{
					Name:        "apiKey",
					Description: "API Key",
					Sensitive:   boolPtr(true),
					Options:     []string{"key1", "key2"},
				},
			},
		},
		{
			name: "fields with empty strings",
			input: gptscript.Fields{
				{Name: "", Description: ""},
				{Name: "field2", Description: ""},
			},
			expected: Fields{
				{Name: "", Description: "", Sensitive: nil, Options: nil},
				{Name: "field2", Description: "", Sensitive: nil, Options: nil},
			},
		},
		{
			name: "empty options slice",
			input: gptscript.Fields{
				{Name: "test", Description: "Test", Options: []string{}},
			},
			expected: Fields{
				{Name: "test", Description: "Test", Sensitive: nil, Options: []string{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToFields(tt.input)

			if len(result) != len(tt.expected) {
				t.Fatalf("ToFields() length = %d; want %d", len(result), len(tt.expected))
			}

			for i := range result {
				if result[i].Name != tt.expected[i].Name {
					t.Errorf("ToFields()[%d].Name = %q; want %q", i, result[i].Name, tt.expected[i].Name)
				}
				if result[i].Description != tt.expected[i].Description {
					t.Errorf("ToFields()[%d].Description = %q; want %q", i, result[i].Description, tt.expected[i].Description)
				}
				if !equalBoolPtr(result[i].Sensitive, tt.expected[i].Sensitive) {
					t.Errorf("ToFields()[%d].Sensitive = %v; want %v", i, formatBoolPtr(result[i].Sensitive), formatBoolPtr(tt.expected[i].Sensitive))
				}
				if !equalStringSlice(result[i].Options, tt.expected[i].Options) {
					t.Errorf("ToFields()[%d].Options = %v; want %v", i, result[i].Options, tt.expected[i].Options)
				}
			}
		})
	}
}

// Helper functions for testing

func boolPtr(b bool) *bool {
	return &b
}

func equalBoolPtr(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func formatBoolPtr(b *bool) string {
	if b == nil {
		return "nil"
	}
	if *b {
		return "true"
	}
	return "false"
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if a == nil && b == nil {
		return true
	}
	if (a == nil) != (b == nil) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
