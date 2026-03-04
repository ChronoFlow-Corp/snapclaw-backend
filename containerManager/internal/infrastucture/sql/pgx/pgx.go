package pgx

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func New(ctx context.Context, url string) (*pgxpool.Pool, error) {

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}

	return pool, nil
}
