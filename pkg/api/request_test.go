package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/obot-platform/obot/apiclient/types"
)

func TestBodyOversizedLeavesRemainderUnread(t *testing.T) {
	const maxBytes = 16
	body := bytes.NewReader(bytes.Repeat([]byte("x"), 1024*1024))
	req := Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/", io.NopCloser(body)),
	}

	_, err := req.Body(BodyOptions{MaxBytes: maxBytes})

	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("Body() error = %v, want HTTP 413", err)
	}

	// Only the limit plus the byte needed to detect overflow should be consumed.
	if consumed := body.Size() - int64(body.Len()); consumed != maxBytes+1 {
		t.Fatalf("Body() consumed %d bytes, want %d", consumed, maxBytes+1)
	}
}

func TestBodyDecodedOversizedLeavesRemainderUnread(t *testing.T) {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()

	const maxBytes = 128
	compressed := encoder.EncodeAll(bytes.Repeat([]byte("x"), maxBytes+1), nil)
	// A second frame leaves valid input after the first frame exceeds the decoded cap.
	compressed = encoder.EncodeAll([]byte("unread remainder"), compressed)
	if len(compressed) > maxBytes {
		t.Fatal("compressed fixture exceeds the raw cap; test must exercise decoded overflow")
	}

	body := bytes.NewReader(compressed)
	req := Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/", io.NopCloser(body)),
	}
	req.Request.Header.Set("Content-Encoding", "zstd")

	_, err = req.Body(BodyOptions{MaxBytes: maxBytes})

	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("Body() error = %v, want HTTP 413", err)
	}
	if body.Len() == 0 {
		t.Fatal("Body() drained the compressed remainder after decoded overflow")
	}
}

func TestBodyLimitsAndEncoding(t *testing.T) {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()

	const maxBytes = 128
	payload := bytes.Repeat([]byte("x"), maxBytes)
	compressed := encoder.EncodeAll(payload, nil)
	compressedOverLimit := encoder.EncodeAll(append(bytes.Clone(payload), 'x'), nil)

	tests := []struct {
		name       string
		body       []byte
		encoding   string
		maxBytes   int64
		want       []byte
		wantCode   int
		wantUnread bool
	}{
		{
			name:     "raw at cap",
			body:     payload,
			maxBytes: maxBytes,
			want:     payload,
		},
		{
			name:     "raw one byte over cap",
			body:     append(bytes.Clone(payload), 'x'),
			maxBytes: maxBytes,
			wantCode: http.StatusRequestEntityTooLarge,
		},
		{
			name:     "identity at cap",
			body:     payload,
			encoding: "identity",
			maxBytes: maxBytes,
			want:     payload,
		},
		{
			name:       "unsupported encoding leaves body unread",
			body:       payload,
			encoding:   "gzip",
			maxBytes:   maxBytes,
			wantCode:   http.StatusUnsupportedMediaType,
			wantUnread: true,
		},
		{
			name:     "zstd decoded at cap",
			body:     compressed,
			encoding: "zstd",
			maxBytes: maxBytes,
			want:     payload,
		},
		{
			name:     "zstd decoded one byte over cap",
			body:     compressedOverLimit,
			encoding: "zstd",
			maxBytes: maxBytes,
			wantCode: http.StatusRequestEntityTooLarge,
		},
		{
			name:     "zstd raw one byte over cap",
			body:     compressed,
			encoding: "zstd",
			maxBytes: int64(len(compressed) - 1),
			wantCode: http.StatusRequestEntityTooLarge,
		},
		{
			name:     "malformed zstd",
			body:     []byte("invalid zstd"),
			encoding: "zstd",
			maxBytes: maxBytes,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := bytes.NewReader(tt.body)
			req := Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        httptest.NewRequest(http.MethodPost, "/", io.NopCloser(body)),
			}
			req.Request.Header.Set("Content-Encoding", tt.encoding)

			got, err := req.Body(BodyOptions{MaxBytes: tt.maxBytes})

			if tt.wantCode == 0 {
				if err != nil {
					t.Fatalf("Body() error = %v", err)
				}
				if !bytes.Equal(got, tt.want) {
					t.Fatalf("Body() = %q, want %q", got, tt.want)
				}
			} else {
				var httpErr *types.ErrHTTP
				if !errors.As(err, &httpErr) || httpErr.Code != tt.wantCode {
					t.Fatalf("Body() error = %v, want HTTP %d", err, tt.wantCode)
				}
			}

			if consumed := len(tt.body) - body.Len(); tt.wantUnread && consumed != 0 {
				t.Fatalf("Body() consumed %d bytes, want 0", consumed)
			}
		})
	}
}
