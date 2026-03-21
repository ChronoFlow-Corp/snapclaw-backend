package controllers

import (
	"context"
	"errors"

	"simpleClaw/internal/entities"

	"github.com/google/uuid"
)

var errUserIDNotFound = errors.New("user id not found in context")

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
