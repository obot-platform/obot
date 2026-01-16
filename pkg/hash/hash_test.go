//nolint:revive
package hash

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "string input",
			input:    "hello world",
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte("hello world"))),
		},
		{
			name:     "empty string",
			input:    "",
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte(""))),
		},
		{
			name:     "byte slice input",
			input:    []byte("hello world"),
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte("hello world"))),
		},
		{
			name:     "empty byte slice",
			input:    []byte{},
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte{})),
		},
		{
			name:  "struct input - marshaled to JSON",
			input: struct{ Name string }{"test"},
			// The hash of JSON: {"Name":"test"}
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte(`{"Name":"test"}`))),
		},
		{
			name:     "map input - marshaled to JSON",
			input:    map[string]string{"key": "value"},
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte(`{"key":"value"}`))),
		},
		{
			name:     "int input - marshaled to JSON",
			input:    42,
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte("42"))),
		},
		{
			name:     "bool input - marshaled to JSON",
			input:    true,
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte("true"))),
		},
		{
			name:     "nil input - marshaled to JSON null",
			input:    nil,
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte("null"))),
		},
		{
			name:     "slice of strings - marshaled to JSON",
			input:    []string{"a", "b", "c"},
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte(`["a","b","c"]`))),
		},
		{
			name:     "nested struct - marshaled to JSON",
			input:    struct{ Inner struct{ Value int } }{Inner: struct{ Value int }{Value: 123}},
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte(`{"Inner":{"Value":123}}`))),
		},
		{
			name:     "unicode string",
			input:    "こんにちは世界",
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte("こんにちは世界"))),
		},
		{
			name:     "string with special characters",
			input:    "hello\nworld\t!",
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte("hello\nworld\t!"))),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := String(tt.input)
			if result != tt.expected {
				t.Errorf("String(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestString_Consistency(t *testing.T) {
	// Test that the same input always produces the same hash
	inputs := []any{
		"test",
		[]byte("test"),
		42,
		struct{ Name string }{"test"},
		map[string]int{"a": 1, "b": 2},
	}

	for _, input := range inputs {
		hash1 := String(input)
		hash2 := String(input)
		if hash1 != hash2 {
			t.Errorf("Hash not consistent for input %v: got %v and %v", input, hash1, hash2)
		}
	}
}

func TestString_DifferentInputsDifferentHashes(t *testing.T) {
	// Test that different inputs produce different hashes
	tests := []struct {
		name   string
		input1 any
		input2 any
	}{
		{
			name:   "different strings",
			input1: "hello",
			input2: "world",
		},
		{
			name:   "different structs",
			input1: struct{ Name string }{"alice"},
			input2: struct{ Name string }{"bob"},
		},
		{
			name:   "different maps",
			input1: map[string]int{"a": 1},
			input2: map[string]int{"a": 2},
		},
		{
			name:   "empty string vs non-empty",
			input1: "",
			input2: "a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := String(tt.input1)
			hash2 := String(tt.input2)
			if hash1 == hash2 {
				t.Errorf("Expected different hashes for %v and %v, but both produced %v", tt.input1, tt.input2, hash1)
			}
		})
	}
}

func TestString_HashFormat(t *testing.T) {
	// Test that the hash is a valid hex string of the correct length (SHA256 is 64 hex chars)
	result := String("test")
	if len(result) != 64 {
		t.Errorf("Hash length = %d, expected 64", len(result))
	}

	// Check that it's all hex characters
	for _, char := range result {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			t.Errorf("Hash contains non-hex character: %c", char)
		}
	}
}
