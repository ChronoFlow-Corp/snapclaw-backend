package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"shared/pkg/observability"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/pkg/slctx"
)

// Logger is a middleware that injects a logger into the request context.
func Logger() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			action, flow := classifySimpleClawActionFlow(r.Method, r.URL.Path)
			ctx := observability.WithComponent(
				observability.WithActionFlow(r.Context(), action, flow),
				"http.server",
			)

			logger := slog.Default()

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

			if skipMetricsLog(r.URL.Path, status) {
				return
			}

			route := RoutePattern(r)

			attrs := []slog.Attr{
				slog.String("result", observabilityResult(status)),
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

			slctx.Logger(r.Context()).LogAttrs(r.Context(), slog.LevelInfo, "request completed", attrs...)
		}

		return http.HandlerFunc(fn)
	}
}

func skipMetricsLog(path string, status int) bool {
	return normalizeActionPath(path) == "/metrics" && status == http.StatusOK
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

func observabilityResult(status int) string {
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return "success"
	case status == http.StatusNotFound:
		return "not_found"
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "denied"
	case status >= http.StatusBadRequest && status < http.StatusInternalServerError:
		return "validation_error"
	default:
		return "error"
	}
}
