package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	jwt2 "shared/pkg/jwt"
	"shared/pkg/response"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/pkg/slctx"
)

type jwtProvider interface {
	ParseAccess(raw string, f any) (jwt2.AccessToken, error)
}

func AuthJwt(j jwtProvider) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			accessCookie, err := r.Cookie("access_token")
			if err != nil {
				slctx.Logger(r.Context()).Debug("No access token cookie", slog.Any("error", err))

				response.RespondError(w, response.Error{
					Code:    http.StatusUnauthorized,
					Message: "Access token required",
				})

				return
			}

			err = accessCookie.Valid()
			if err != nil {
				slctx.Logger(r.Context()).Debug("Access token is invalid", slog.Any("error", err))

				response.RespondError(w, response.Error{
					Code:    http.StatusUnauthorized,
					Message: "Access token is invalid",
				})

				return
			}

			raw := accessCookie.Value
			if raw == "" {
				slctx.Logger(r.Context()).Debug("No Authorization header")
				response.RespondError(
					w,
					response.Error{
						Code:    http.StatusUnauthorized,
						Message: "Authorization required",
					},
				)

				return
			}

			token, err := j.ParseAccess(raw, jwt.ParseRSAPublicKeyFromPEM)
			if err != nil {
				if errors.Is(err, jwt2.ErrExpired) {
					slctx.Logger(r.Context()).Debug("Token expired")
					response.RespondError(
						w,
						response.Error{Code: http.StatusUnauthorized, Message: "Token expired"},
					)
				}

				if errors.Is(err, jwt2.ErrInvalid) {
					slctx.Logger(r.Context()).Debug("Token invalid")
					response.RespondError(
						w,
						response.Error{Code: http.StatusUnauthorized, Message: "Token invalid"},
					)
				}

				slctx.Logger(r.Context()).Error("Token parse error", slog.Any("err", err))

				return
			}

			ctx := context.WithValue(r.Context(), entities.UserIDCtxKey{}, token.Claims.UserID)
			ctx = context.WithValue(ctx, entities.SessionIDCtxKey{}, token.Claims.SessionID)

			logger := slctx.Logger(ctx).With(
				slog.String("user_id", token.Claims.UserID.String()),
				slog.String("session_id", token.Claims.SessionID.String()),
			)
			ctx = slctx.WithLogger(ctx, logger)

			r = r.WithContext(ctx)
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
		}

		return http.HandlerFunc(fn)
	}
}
