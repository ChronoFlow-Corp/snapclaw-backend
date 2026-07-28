package middleware

import (
	"log/slog"
	"net/http"
)

func Auth(key string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if key == "" {
				slog.Default().Warn("Api key empty")

				next.ServeHTTP(w, r)
			}
			if h == "" {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

				return
			}

			if h != key {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

				return
			}

			next.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
