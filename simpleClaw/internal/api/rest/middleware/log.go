package middleware

import (
	"log/slog"
	"net/http"

	"simpleClaw/internal/pkg/slctx"

	"simpleClaw/internal/entities"

	"github.com/go-chi/chi/v5/middleware"
)

// Logger is a middleware that injects a logger into the request context.
func Logger() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			logger := slog.Default()

			userID, ok := r.Context().Value(entities.UserIDCtxKey{}).(string)
			if ok && userID != "" {
				logger = logger.With("user_id", userID)
			}

			ctx := slctx.WithLogger(
				r.Context(),
				logger.With("request_id", middleware.GetReqID(r.Context())),
			)
			r = r.WithContext(ctx)
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
		}

		return http.HandlerFunc(fn)
	}
}
