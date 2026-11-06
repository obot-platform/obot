package llmcore

import (
	"fmt"
	"io"
	"net/http"

	"github.com/maximhq/bifrost/core/schemas"
)

const (
	responseEncodingMessage = "failed to encode gateway response"
)

// streamWriter owns header commitment for every streaming dialect. Encoding must
// succeed before the first event commits HTTP 200; after that, failures can only
// be reported as SSE errors. Callers stop and cancel the producer on any error.
type streamWriter struct {
	w       http.ResponseWriter
	dialect dialect
	started bool
	// Generated Responses errors must follow the provider's last event number.
	nextSequenceNumber int
}

func (s *streamWriter) write(name string, value any, extra schemas.BifrostResponseExtraFields) error {
	data, err := marshal(value)
	if err != nil {
		s.fail(&schemas.BifrostError{
			StatusCode: schemas.Ptr(http.StatusInternalServerError),
			Error:      &schemas.ErrorField{Type: new("api_error"), Message: responseEncodingMessage},
		})
		return err
	}
	if !s.started {
		setServed(s.w.Header(), extra)
		s.w.Header().Set("Content-Type", "text/event-stream")
		s.w.Header().Set("Cache-Control", "no-cache")
		s.w.WriteHeader(http.StatusOK)
		s.started = true
	}
	// A transport failure means the connection is unusable. Return directly;
	// attempting an error event or flush would write to the failed transport.
	if err := writeEncodedEvent(s.w, name, data); err != nil {
		return err
	}
	if s.dialect == dialectResponses {
		if event, ok := value.(*schemas.BifrostResponsesStreamResponse); ok {
			s.nextSequenceNumber = max(s.nextSequenceNumber, event.SequenceNumber+1)
		}
	}
	return http.NewResponseController(s.w).Flush()
}

func (s *streamWriter) fail(err *schemas.BifrostError) {
	if !s.started {
		writeEngineError(s.w, s.dialect, err)
		return
	}
	if err := writeStreamError(s.w, s.dialect, err, s.nextSequenceNumber); err != nil {
		return
	}
	_ = http.NewResponseController(s.w).Flush()
}

func (s *streamWriter) finishChat() error {
	if !s.started {
		return nil
	}
	if _, err := io.WriteString(s.w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	return http.NewResponseController(s.w).Flush()
}

// A complete encoded event is written at once, so serialization errors cannot
// leave an event prefix in the response before error handling begins.
func writeEncodedEvent(w http.ResponseWriter, name string, data []byte) error {
	if name != "" {
		_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
		return err
	}
	_, err := fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}
