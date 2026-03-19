package middleware

import (
	"log/slog"
	"net/http"
	"shared/pkg/observability"
	"strings"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/pkg/slctx"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Logger is a middleware that injects a logger into the request context.
func Logger() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			action, flow := classifySimpleClawActionFlow(r.Method, r.URL.Path)
			ctx := observability.WithActionFlow(r.Context(), action, flow)

			logger := observability.EnrichLogger(ctx, slog.Default())

			switch userID := ctx.Value(entities.UserIDCtxKey{}).(type) {
			case string:
				if userID != "" {
					logger = logger.With("user_id", userID)
				}
			case interface{ String() string }:
				logger = logger.With("user_id", userID.String())
			}

			ctx = slctx.WithLogger(
				ctx,
				logger.With("request_id", middleware.GetReqID(r.Context())),
			)
			r = r.WithContext(ctx)
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()

			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			route := RoutePattern(r)

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("route", route),
				slog.Int("status", status),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("remote_ip", r.RemoteAddr),
			}
			if ua := r.UserAgent(); ua != "" {
				attrs = append(attrs, slog.String("user_agent", ua))
			}

			slctx.Logger(r.Context()).Info("request completed", slog.GroupAttrs("attrs", attrs...))
		}

		return http.HandlerFunc(fn)
	}
}

func RoutePattern(r *http.Request) string {
	if r == nil {
		return "unknown"
	}

	rctx := chi.RouteContext(r.Context())
	if rctx != nil {
		if pattern := strings.TrimSpace(rctx.RoutePattern()); pattern != "" {
			return pattern
		}
	}

	return normalizeActionPath(r.URL.Path)
}
