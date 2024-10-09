package context

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

type reqIDKey struct{}

func WithNewRequestID(ctx context.Context) context.Context {
	return context.WithValue(ctx, reqIDKey{}, uuid.NewString())
}

func GetRequestID(ctx context.Context) string {
	s, _ := ctx.Value(reqIDKey{}).(string)
	return s
}

type loggerKey struct{}

func WithLogger(ctx context.Context, log *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, log)
}

func GetLogger(ctx context.Context) *slog.Logger {
	l, ok := ctx.Value(loggerKey{}).(*slog.Logger)
	if !ok || l == nil {
		return slog.Default()
	}

	return l
}
