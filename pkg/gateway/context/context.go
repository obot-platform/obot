package context

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/gptscript-ai/otto/pkg/gateway/types"
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

type userKey struct{}

func WithUser(ctx context.Context, user *types.User) context.Context {
	return context.WithValue(ctx, userKey{}, user)
}

func GetUser(ctx context.Context) *types.User {
	u, _ := ctx.Value(userKey{}).(*types.User)
	return u
}

type identityKey struct{}

func WithIdentity(ctx context.Context, identity *types.Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

func GetIdentity(ctx context.Context) *types.Identity {
	i, _ := ctx.Value(identityKey{}).(*types.Identity)
	return i
}
