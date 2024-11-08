package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

func writeResponse(ctx context.Context, logger *slog.Logger, w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		writeError(ctx, logger, w, http.StatusInternalServerError, fmt.Errorf("failed to marshal response: %w", err))
		return
	}

	_, _ = w.Write(b)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeError(ctx context.Context, logger *slog.Logger, w http.ResponseWriter, code int, err error) {
	logger.DebugContext(ctx, "Writing error response", "code", code, "error", err)

	w.WriteHeader(code)
	resp := map[string]any{
		"error": err.Error(),
	}

	b, err := json.Marshal(resp)
	if err != nil {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err.Error())))
		return
	}

	_, _ = w.Write(b)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) authCompleteURL() string {
	return fmt.Sprintf("%s/login_complete", s.uiURL)
}
