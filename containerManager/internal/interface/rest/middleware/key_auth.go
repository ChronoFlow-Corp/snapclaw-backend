package middleware

import "net/http"

func Auth(key string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if h == "" {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			}

			if h != key {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			}

			next.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
