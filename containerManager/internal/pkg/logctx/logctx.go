package logctx

import (
	"context"
	"log/slog"
	"shared/pkg/observability"
)

type ctxLoggerKey struct{}

func Logger(ctx context.Context) *slog.Logger {
	l, ok := ctx.Value(ctxLoggerKey{}).(*slog.Logger)
	if !ok {
		return observability.EnrichLogger(ctx, slog.Default())
	}

	return observability.EnrichLogger(ctx, l)
}

func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxLoggerKey{}, l)
}
