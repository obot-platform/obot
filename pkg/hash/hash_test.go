//nolint:revive
package hash

import (
	"testing"
)

func TestString_KnownValues(t *testing.T) {
	// Test with known SHA256 hashes (pre-computed and verified)
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "hello world string",
			input:    "hello world",
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "hello world bytes",
			input:    []byte("hello world"),
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
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

func TestString_JSONMarshaledTypes(t *testing.T) {
	// Test that non-string/non-byte types are JSON marshaled before hashing
	// The hash of JSON "42" should be different from the hash of string "42"
	intHash := String(42)
	strHash := String("42")

	// These should be the same because JSON marshal of 42 is "42"
	if intHash != strHash {
		t.Errorf("int 42 and string \"42\" should produce same hash after JSON marshaling, got %v and %v", intHash, strHash)
	}

	// Nil should marshal to "null"
	nilHash := String(nil)
	nullStrHash := String("null")
	if nilHash != nullStrHash {
		t.Errorf("nil and string \"null\" should produce same hash, got %v and %v", nilHash, nullStrHash)
	}
}
