package slctx

import (
	"context"
	"log/slog"
)

type ctxLoggerKey struct{}

func Logger(ctx context.Context) *slog.Logger {
	l, ok := ctx.Value(ctxLoggerKey{}).(*slog.Logger)
	if !ok {
		return slog.Default()
	}

	return l
}

func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxLoggerKey{}, l)
}
