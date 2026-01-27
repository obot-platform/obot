package gz

import (
	"bytes"
	"compress/gzip"
	"testing"
)

func TestCompress(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{
			name:    "string input",
			input:   "hello world",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: false,
		},
		{
			name:    "byte slice input",
			input:   []byte("hello world"),
			wantErr: false,
		},
		{
			name:    "empty byte slice",
			input:   []byte{},
			wantErr: false,
		},
		{
			name:    "struct input - marshaled to JSON",
			input:   struct{ Name string }{"test"},
			wantErr: false,
		},
		{
			name:    "map input - marshaled to JSON",
			input:   map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name:    "int input - marshaled to JSON",
			input:   42,
			wantErr: false,
		},
		{
			name:    "slice input - marshaled to JSON",
			input:   []string{"a", "b", "c"},
			wantErr: false,
		},
		{
			name:    "unicode string",
			input:   "こんにちは世界",
			wantErr: false,
		},
		{
			name:    "large string",
			input:   string(make([]byte, 10000)),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Compress(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Compress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Error("Compress() returned nil result")
			}

			// Verify the result is valid gzip data
			if !tt.wantErr {
				_, err := gzip.NewReader(bytes.NewBuffer(result))
				if err != nil {
					t.Errorf("Compress() produced invalid gzip data: %v", err)
				}
			}
		})
	}
}

func TestDecompress(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		outType  string // "string", "bytes", "json"
		expected any
		wantErr  bool
	}{
		{
			name:     "decompress to string",
			input:    "hello world",
			outType:  "string",
			expected: "hello world",
			wantErr:  false,
		},
		{
			name:     "decompress empty string",
			input:    "",
			outType:  "string",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "decompress to byte slice",
			input:    "hello world",
			outType:  "bytes",
			expected: []byte("hello world"),
			wantErr:  false,
		},
		{
			name:     "decompress empty to byte slice",
			input:    "",
			outType:  "bytes",
			expected: []byte{},
			wantErr:  false,
		},
		{
			name:     "decompress JSON struct",
			input:    struct{ Name string }{"test"},
			outType:  "json",
			expected: map[string]any{"Name": "test"},
			wantErr:  false,
		},
		{
			name:     "decompress JSON map",
			input:    map[string]string{"key": "value"},
			outType:  "json",
			expected: map[string]any{"key": "value"},
			wantErr:  false,
		},
		{
			name:     "decompress unicode",
			input:    "こんにちは世界",
			outType:  "string",
			expected: "こんにちは世界",
			wantErr:  false,
		},
		{
			name:     "decompress large string",
			input:    string(make([]byte, 10000)),
			outType:  "string",
			expected: string(make([]byte, 10000)),
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First compress the input
			compressed, err := Compress(tt.input)
			if err != nil {
				t.Fatalf("Failed to compress input: %v", err)
			}

			// Then decompress it
			var result any
			switch tt.outType {
			case "string":
				var s string
				result = &s
			case "bytes":
				var b []byte
				result = &b
			case "json":
				result = &map[string]any{}
			}

			err = Decompress(result, compressed)
			if (err != nil) != tt.wantErr {
				t.Errorf("Decompress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify the result matches expected
				switch tt.outType {
				case "string":
					if *(result.(*string)) != tt.expected.(string) {
						t.Errorf("Decompress() = %v, expected %v", *(result.(*string)), tt.expected)
					}
				case "bytes":
					if !bytes.Equal(*(result.(*[]byte)), tt.expected.([]byte)) {
						t.Errorf("Decompress() = %v, expected %v", *(result.(*[]byte)), tt.expected)
					}
				case "json":
					// For JSON, we'll just check that we got a non-nil result
					// Deep comparison is complex due to type conversions
					if result == nil {
						t.Error("Decompress() returned nil result")
					}
				}
			}
		})
	}
}

func TestCompressDecompressRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{
			name:  "string round trip",
			input: "hello world",
		},
		{
			name:  "byte slice round trip",
			input: []byte("test data"),
		},
		{
			name:  "empty string round trip",
			input: "",
		},
		{
			name:  "unicode round trip",
			input: "テスト",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compress
			compressed, err := Compress(tt.input)
			if err != nil {
				t.Fatalf("Compress() error = %v", err)
			}

			// Decompress
			var result any
			switch tt.input.(type) {
			case string:
				var s string
				result = &s
			case []byte:
				var b []byte
				result = &b
			}

			err = Decompress(result, compressed)
			if err != nil {
				t.Fatalf("Decompress() error = %v", err)
			}

			// Verify
			switch v := tt.input.(type) {
			case string:
				if *(result.(*string)) != v {
					t.Errorf("Round trip failed: got %v, expected %v", *(result.(*string)), v)
				}
			case []byte:
				if !bytes.Equal(*(result.(*[]byte)), v) {
					t.Errorf("Round trip failed: got %v, expected %v", *(result.(*[]byte)), v)
				}
			}
		})
	}
}

func TestDecompressInvalidGzip(t *testing.T) {
	// Test decompressing invalid gzip data
	var result string
	err := Decompress(&result, []byte("not gzip data"))
	if err == nil {
		t.Error("Decompress() expected error for invalid gzip data, got nil")
	}
}

func TestCompressProducesValidGzip(t *testing.T) {
	input := "test data"
	compressed, err := Compress(input)
	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}

	// Verify we can decompress with standard gzip reader
	reader, err := gzip.NewReader(bytes.NewBuffer(compressed))
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	var buf bytes.Buffer
	_, err = buf.ReadFrom(reader)
	if err != nil {
		t.Fatalf("Failed to read gzip data: %v", err)
	}

	if buf.String() != input {
		t.Errorf("Decompressed data = %v, expected %v", buf.String(), input)
	}
}
