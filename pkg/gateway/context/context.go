//nolint:revive
package context

import (
	"context"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/logger"
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

func WithLogger(ctx context.Context, log *logger.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, log)
}

func GetLogger(ctx context.Context) *logger.Logger {
	l, ok := ctx.Value(loggerKey{}).(*logger.Logger)
	if !ok || l == nil {
		log := logger.New("")
		return &log
	}

	return l
}
