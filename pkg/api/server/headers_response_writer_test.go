package server

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

type countingResponseWriter struct {
	hdr http.Header

	code                 int
	writeHeaderCalls     int
	writeCalls           int
	readFromCalls        int
	headersAtWriteHeader http.Header

	body bytes.Buffer
}

func newCountingResponseWriter() *countingResponseWriter {
	return &countingResponseWriter{hdr: make(http.Header)}
}

func (w *countingResponseWriter) Header() http.Header {
	return w.hdr
}

func (w *countingResponseWriter) WriteHeader(status int) {
	w.writeHeaderCalls++
	if w.code == 0 {
		w.code = status
		w.headersAtWriteHeader = cloneHeader(w.hdr)
	}
}

func (w *countingResponseWriter) Write(p []byte) (int, error) {
	w.writeCalls++
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(p)
}

func (w *countingResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	w.readFromCalls++
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.ReadFrom(r)
}

func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, vv := range h {
		cp := make([]string, len(vv))
		copy(cp, vv)
		out[k] = cp
	}
	return out
}

func TestHeadersResponseWriter_SetsSecurityHeadersAndContentType(t *testing.T) {
	t.Parallel()

	bodyStatuses := []int{
		http.StatusOK,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}
	noBodyStatuses := []int{
		http.StatusContinue,
		http.StatusSwitchingProtocols,
		http.StatusNoContent,
		http.StatusResetContent,
		http.StatusNotModified,
	}

	for _, status := range append(bodyStatuses, noBodyStatuses...) {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			crw := newCountingResponseWriter()
			rw := &headersResponseWriter{ResponseWriter: crw}
			rw.WriteHeader(status)

			if got := crw.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Fatalf("X-Content-Type-Options = %q, want %q", got, "nosniff")
			}

			expectContentType := ""
			for _, s := range bodyStatuses {
				if s == status {
					expectContentType = "text/plain; charset=utf-8"
					break
				}
			}
			if got := crw.Header().Get("Content-Type"); got != expectContentType {
				t.Fatalf("Content-Type = %q, want %q (status=%d)", got, expectContentType, status)
			}
		})
	}
}

func TestHeadersResponseWriter_DoesNotOverwriteContentType(t *testing.T) {
	t.Parallel()

	crw := newCountingResponseWriter()
	crw.Header().Set("Content-Type", "application/json")

	rw := &headersResponseWriter{ResponseWriter: crw}
	rw.WriteHeader(http.StatusOK)

	if got := crw.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", got, "application/json")
	}
	if got := crw.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}

	if crw.writeHeaderCalls != 1 {
		t.Fatalf("underlying WriteHeader calls = %d, want 1", crw.writeHeaderCalls)
	}
	if got := crw.headersAtWriteHeader.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type at underlying WriteHeader = %q, want %q", got, "application/json")
	}
}

func TestHeadersResponseWriter_WriteImplicitlyCallsWriteHeaderOnce(t *testing.T) {
	t.Parallel()

	crw := newCountingResponseWriter()
	rw := &headersResponseWriter{ResponseWriter: crw}

	if _, err := rw.Write([]byte("hi")); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if crw.writeHeaderCalls != 1 {
		t.Fatalf("underlying WriteHeader calls = %d, want 1", crw.writeHeaderCalls)
	}
	if crw.code != http.StatusOK {
		t.Fatalf("status = %d, want %d", crw.code, http.StatusOK)
	}

	if got := crw.headersAtWriteHeader.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options at underlying WriteHeader = %q, want %q", got, "nosniff")
	}
	if got := crw.headersAtWriteHeader.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type at underlying WriteHeader = %q, want %q", got, "text/plain; charset=utf-8")
	}
	if got := crw.body.String(); got != "hi" {
		t.Fatalf("body = %q, want %q", got, "hi")
	}
}

func TestHeadersResponseWriter_ReadFromUsesUnderlyingReaderFrom(t *testing.T) {
	t.Parallel()

	crw := newCountingResponseWriter()
	rw := &headersResponseWriter{ResponseWriter: crw}

	n, err := rw.ReadFrom(strings.NewReader("abc"))
	if err != nil {
		t.Fatalf("ReadFrom error: %v", err)
	}
	if n != 3 {
		t.Fatalf("ReadFrom bytes = %d, want 3", n)
	}
	if crw.readFromCalls != 1 {
		t.Fatalf("underlying ReadFrom calls = %d, want 1", crw.readFromCalls)
	}
	if crw.writeHeaderCalls != 1 || crw.code != http.StatusOK {
		t.Fatalf("underlying WriteHeader calls/status = %d/%d, want 1/%d", crw.writeHeaderCalls, crw.code, http.StatusOK)
	}
	if got := crw.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	if got := crw.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/plain; charset=utf-8")
	}
	if got := crw.body.String(); got != "abc" {
		t.Fatalf("body = %q, want %q", got, "abc")
	}
}

type noReaderFromResponseWriter struct {
	hdr http.Header

	code                 int
	writeHeaderCalls     int
	writeCalls           int
	headersAtWriteHeader http.Header

	body bytes.Buffer
}

func newNoReaderFromResponseWriter() *noReaderFromResponseWriter {
	return &noReaderFromResponseWriter{hdr: make(http.Header)}
}

func (w *noReaderFromResponseWriter) Header() http.Header {
	return w.hdr
}

func (w *noReaderFromResponseWriter) WriteHeader(status int) {
	w.writeHeaderCalls++
	if w.code == 0 {
		w.code = status
		w.headersAtWriteHeader = cloneHeader(w.hdr)
	}
}

func (w *noReaderFromResponseWriter) Write(p []byte) (int, error) {
	w.writeCalls++
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(p)
}

func TestHeadersResponseWriter_ReadFromFallsBackToCopyWhenUnsupported(t *testing.T) {
	t.Parallel()

	crw := newNoReaderFromResponseWriter()
	rw := &headersResponseWriter{ResponseWriter: crw}

	n, err := rw.ReadFrom(strings.NewReader("abcd"))
	if err != nil {
		t.Fatalf("ReadFrom error: %v", err)
	}
	if n != 4 {
		t.Fatalf("ReadFrom bytes = %d, want 4", n)
	}
	if crw.writeCalls == 0 {
		t.Fatalf("expected fallback to Write via io.Copy")
	}
	if got := crw.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	if got := crw.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/plain; charset=utf-8")
	}
}

func TestHeadersResponseWriter_WroteHeaderPreventsDuplicateWriteHeaderCalls(t *testing.T) {
	t.Parallel()

	crw := newCountingResponseWriter()
	rw := &headersResponseWriter{ResponseWriter: crw}

	rw.WriteHeader(http.StatusOK)
	rw.WriteHeader(http.StatusNotFound)
	if _, err := rw.Write([]byte("x")); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if crw.writeHeaderCalls != 1 {
		t.Fatalf("underlying WriteHeader calls = %d, want 1", crw.writeHeaderCalls)
	}
	if crw.code != http.StatusOK {
		t.Fatalf("status = %d, want %d", crw.code, http.StatusOK)
	}
	if got := crw.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
}

func TestHeadersResponseWriter_WriteAfterWriteHeaderPreservesStatusAndHeaders(t *testing.T) {
	t.Parallel()

	crw := newCountingResponseWriter()
	rw := &headersResponseWriter{ResponseWriter: crw}

	rw.WriteHeader(http.StatusNotFound)
	if _, err := rw.Write([]byte("nope")); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if crw.writeHeaderCalls != 1 {
		t.Fatalf("underlying WriteHeader calls = %d, want 1", crw.writeHeaderCalls)
	}
	if crw.code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", crw.code, http.StatusNotFound)
	}
	if got := crw.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	if got := crw.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/plain; charset=utf-8")
	}
}
