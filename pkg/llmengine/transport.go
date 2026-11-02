package llmengine

import (
	"io"
	"net/http"
	"sync"
)

// handlerTransport streams an in-process handler through a pipe. Closing the response
// body cancels the request and unblocks a writer after client disconnects.
type handlerTransport struct{ handler http.Handler }

type pipeResponse struct {
	header, sent http.Header
	status       int
	writer       *io.PipeWriter
	once         sync.Once
	ready        chan struct{}
}

func (t handlerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	reader, writer := io.Pipe()
	w := &pipeResponse{header: make(http.Header), writer: writer, ready: make(chan struct{})}
	go func() {
		defer func() {
			if recover() != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = writer.CloseWithError(io.ErrUnexpectedEOF)
			}
		}()
		t.handler.ServeHTTP(w, r)
		w.WriteHeader(http.StatusOK)
		_ = writer.Close()
	}()
	select {
	case <-r.Context().Done():
		_ = reader.CloseWithError(r.Context().Err())
		return nil, r.Context().Err()
	case <-w.ready:
		return &http.Response{StatusCode: w.status, Status: http.StatusText(w.status), Header: w.sent, Body: reader, Request: r, ContentLength: -1}, nil
	}
}

func (w *pipeResponse) Header() http.Header { return w.header }
func (w *pipeResponse) WriteHeader(status int) {
	w.once.Do(func() { w.status = status; w.sent = w.header.Clone(); close(w.ready) })
}
func (w *pipeResponse) Write(data []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.writer.Write(data)
}
func (w *pipeResponse) Flush() { w.WriteHeader(http.StatusOK) }
