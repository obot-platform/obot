package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/api/server/audit"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type flushErrorWriter struct {
	*countingResponseWriter
	flushes int
	err     error
}

type flushAuditLogger struct {
	entries []audit.LogEntry
}

// The real HTTP writer exposes both methods; tracing used to lose the error via Flush.
type legacyFlushErrorWriter struct {
	*flushErrorWriter
}

func (w *flushErrorWriter) FlushError() error {
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	w.flushes++
	return w.err
}

func (l *flushAuditLogger) LogEntry(entry audit.LogEntry) error {
	l.entries = append(l.entries, entry)
	return nil
}

func (*flushAuditLogger) Close() error { return nil }

func TestResponseWritersFlush(t *testing.T) {
	for _, wrapper := range []string{"headers", "audit", "audit over headers"} {
		t.Run(wrapper, func(t *testing.T) {
			logger := &flushAuditLogger{}
			wrap := func(w http.ResponseWriter) http.ResponseWriter {
				if wrapper != "audit" {
					w = &headersResponseWriter{ResponseWriter: w}
				}
				if wrapper != "headers" {
					w = &responseWriter{ResponseWriter: w, auditLogger: logger}
				}
				return w
			}

			// This writer has FlushError only, so delegation must use ResponseController.
			underlying := &flushErrorWriter{countingResponseWriter: newCountingResponseWriter(), err: io.ErrClosedPipe}
			w := wrap(underlying)
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusAccepted)
			require.ErrorIs(t, http.NewResponseController(w).Flush(), io.ErrClosedPipe)
			require.Equal(t, 1, underlying.flushes)

			// Legacy callers still get http.Flusher, whose signature cannot return errors.
			require.Implements(t, (*http.Flusher)(nil), w)
			w.(http.Flusher).Flush()
			require.Equal(t, 2, underlying.flushes)
			require.Equal(t, 1, underlying.writeHeaderCalls)
			require.Equal(t, http.StatusAccepted, underlying.code)
			require.Equal(t, "text/event-stream", underlying.headersAtWriteHeader.Get("Content-Type"))
			if wrapper != "audit" {
				require.Equal(t, "nosniff", underlying.headersAtWriteHeader.Get("X-Content-Type-Options"))
			}
			if wrapper != "headers" {
				require.Len(t, logger.entries, 1)
				require.Equal(t, http.StatusAccepted, logger.entries[0].ResponseCode)
			}

			legacy := httptest.NewRecorder()
			require.NoError(t, http.NewResponseController(wrap(legacy)).Flush())
			require.True(t, legacy.Flushed)
			require.ErrorIs(t, http.NewResponseController(wrap(newCountingResponseWriter())).Flush(), http.ErrNotSupported)
		})
	}
}

func (w *legacyFlushErrorWriter) Flush() { _ = w.FlushError() }

func TestTracedFlushPreservesErrorAndTelemetry(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	for _, path := range []string{"/api/stream", "/api/healthz", "/_app/immutable/app.js"} {
		for _, body := range []string{"none", "write", "read-from"} {
			for _, flushErr := range []error{nil, io.ErrClosedPipe} {
				t.Run(fmt.Sprintf("%s/body=%s/error=%v", path, body, flushErr), func(t *testing.T) {
					exporter.Reset()
					underlying := &legacyFlushErrorWriter{flushErrorWriter: &flushErrorWriter{
						countingResponseWriter: newCountingResponseWriter(),
						err:                    flushErr,
					}}
					status, size := http.StatusOK, int64(0)
					handler := traced(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w = &headersResponseWriter{ResponseWriter: w}
						r.Pattern = path
						if body != "none" {
							status, size = http.StatusAccepted, 2
							w.WriteHeader(status)
							if body == "read-from" {
								_, err := w.(*headersResponseWriter).ReadFrom(strings.NewReader("hi"))
								require.NoError(t, err)
							} else {
								_, err := w.Write([]byte("hi"))
								require.NoError(t, err)
							}
						}
						err := http.NewResponseController(w).Flush()
						if flushErr == nil {
							require.NoError(t, err)
						} else {
							require.ErrorIs(t, err, flushErr)
						}
						// Flush commits the initial status even when no body preceded it.
						w.WriteHeader(http.StatusTeapot)
					}))
					handler.ServeHTTP(underlying, httptest.NewRequest(http.MethodGet, path, nil))
					require.Equal(t, 1, underlying.flushes)
					require.Equal(t, status, underlying.code)
					require.Equal(t, 1, underlying.writeHeaderCalls, "flush preserves header commitment")
					require.Equal(t, "nosniff", underlying.headersAtWriteHeader.Get("X-Content-Type-Options"))
					spans := exporter.GetSpans()
					if path != "/api/stream" {
						require.Empty(t, spans, "health checks and static assets remain untraced")
						return
					}
					require.Len(t, spans, 1)
					require.Equal(t, path, spans[0].Name)
					attributes := map[string]int64{}
					for _, attribute := range spans[0].Attributes {
						attributes[string(attribute.Key)] = attribute.Value.AsInt64()
					}
					require.Equal(t, int64(status), attributes["http.response.status_code"])
					require.Equal(t, size, attributes["http.response.body.size"])
				})
			}
		}
	}
}
