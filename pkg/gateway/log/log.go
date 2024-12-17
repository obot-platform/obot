package log

import (
	"context"
	"log/slog"

	kcontext "github.com/obot-platform/obot/pkg/gateway/context"
)

func New() *slog.Logger {
	return NewWithID("")
}

func NewWithID(id string) *slog.Logger {
	return slog.New(&handler{
		h:  slog.Default().Handler(),
		id: id,
	})
}

type handler struct {
	id string
	h  slog.Handler
}

func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.h.Enabled(ctx, level)
}

func (h *handler) Handle(ctx context.Context, record slog.Record) error {
	if h.id != "" {
		record.AddAttrs(slog.String("req_id", h.id))
	} else if id := kcontext.GetRequestID(ctx); id != "" {
		record.AddAttrs(slog.String("req_id", id))
	}

	return h.h.Handle(ctx, record)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &handler{h: h.h.WithAttrs(attrs)}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{h: h.h.WithGroup(name)}
}
