package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"shared/pkg/response"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
)

type adminUserProvider interface {
	UserInfo(ctx context.Context, userID uuid.UUID) (entities.User, error)
}

func AdminOnly(users adminUserProvider) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := userIDFromContext(r.Context())
			if err != nil {
				response.RespondError(w, response.Error{
					Code:    http.StatusUnauthorized,
					Message: "invalid user id",
				})

				return
			}

			user, err := users.UserInfo(r.Context(), userID)
			if err != nil {
				code := http.StatusInternalServerError
				message := "internal error"

				if errors.Is(err, sql.ErrNotFound) {
					code = http.StatusUnauthorized
					message = "user not found"
				}

				response.RespondError(w, response.Error{
					Code:    code,
					Message: message,
				})

				return
			}

			if user.Role != entities.AdminRole {
				response.RespondError(w, response.Error{
					Code:    http.StatusForbidden,
					Message: "admin access required",
				})

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func userIDFromContext(ctx context.Context) (uuid.UUID, error) {
	value := ctx.Value(entities.UserIDCtxKey{})
	switch v := value.(type) {
	case uuid.UUID:
		if v == uuid.Nil {
			return uuid.Nil, errUserIDNotFound
		}

		return v, nil
	case string:
		if v == "" {
			return uuid.Nil, errUserIDNotFound
		}

		id, err := uuid.Parse(v)
		if err != nil {
			return uuid.Nil, err
		}

		return id, nil
	default:
		return uuid.Nil, errUserIDNotFound
	}
}

var errUserIDNotFound = errors.New("user id not found in context")
