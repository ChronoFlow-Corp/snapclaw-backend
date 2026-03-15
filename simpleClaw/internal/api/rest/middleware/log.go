package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/pkg/slctx"

	"github.com/go-chi/chi/v5/middleware"
)

// Logger is a middleware that injects a logger into the request context.
func Logger() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			logger := slog.Default()

			switch userID := r.Context().Value(entities.UserIDCtxKey{}).(type) {
			case string:
				if userID != "" {
					logger = logger.With("user_id", userID)
				}
			case interface{ String() string }:
				logger = logger.With("user_id", userID.String())
			}

			ctx := slctx.WithLogger(
				r.Context(),
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

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("remote_ip", r.RemoteAddr),
			}
			if rawQuery := r.URL.RawQuery; rawQuery != "" {
				attrs = append(attrs, slog.String("query", rawQuery))
			}
			if ua := r.UserAgent(); ua != "" {
				attrs = append(attrs, slog.String("user_agent", ua))
			}

			slctx.Logger(r.Context()).Info("request completed", slog.GroupAttrs("attrs", attrs...))
		}

		return http.HandlerFunc(fn)
	}
}
