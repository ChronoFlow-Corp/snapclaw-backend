package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"containermanager/internal/entities"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	table = "containers"
)

const (
	id = iota
	userID
	containerID
	status
	port
	createdAt
	updatedAt
)

var columns = []string{
	id:          "id",
	userID:      "user_id",
	containerID: "container_id",
	status:      "status",
	port:        "port",
	createdAt:   "created_at",
	updatedAt:   "updated_at",
}

type Container struct {
	pool *pgxpool.Pool
}

var sq = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

func NewContainer(pool *pgxpool.Pool) *Container {
	return &Container{pool: pool}
}

func (c *Container) Create(ctx context.Context, cl entities.Container) error {
	const op = "storage.Container.Create"

	query, values, err := sq.Insert(table).
		Columns(columns[id], columns[containerID], columns[port], columns[userID]).
		Values(cl.ID, cl.ContainerID, cl.Port, cl.UserID).
		ToSql()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	_, err = c.pool.Exec(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (c *Container) Update(ctx context.Context, cl entities.Container) error {
	const op = "storage.Container.Update"

	cDb, err := c.GetByUserID(ctx, cl.UserID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	b := sq.Update(table).Where(squirrel.Eq{columns[id]: cl.ID})

	if cDb.Status != cl.Status {
		b = b.Set(columns[status], cl.Status)
	}

	if cDb.Port != cl.Port {
		b = b.Set(columns[port], cl.Port)
	}

	b = b.Set(columns[updatedAt], time.Now())

	query, values, err := b.ToSql()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	_, err = c.pool.Exec(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (c *Container) Remove(ctx context.Context, cl entities.Container) error {
	const op = "storage.Container.Remove"

	query, values, err := sq.Delete(table).Where(squirrel.Eq{columns[id]: cl.ID}).ToSql()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	_, err = c.pool.Exec(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (c *Container) GetByUserID(ctx context.Context, uID string) (entities.Container, error) {
	const op = "storage.Container.GetByUserID"

	query, values, err := sq.Select(columns...).
		From(table).
		Where(squirrel.Eq{columns[userID]: uID}).
		ToSql()
	if err != nil {
		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	var res entities.Container

	row := c.pool.QueryRow(ctx, query, values...)

	err = row.Scan(
		&res.ID,
		&res.UserID,
		&res.ContainerID,
		&res.Status,
		&res.Port,
		&res.CreatedAt,
		&res.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entities.Container{}, fmt.Errorf("%s: %w", op, ErrNotFound)
		}

		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	return res, nil
}

func (c *Container) GetByID(ctx context.Context, containerID string) (entities.Container, error) {
	const op = "storage.Container.GetByID"

	query, values, err := sq.Select(columns...).
		From(table).
		Where(squirrel.Eq{columns[id]: containerID}).
		ToSql()
	if err != nil {
		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	var res entities.Container

	row := c.pool.QueryRow(ctx, query, values...)

	err = row.Scan(
		&res.ID,
		&res.UserID,
		&res.ContainerID,
		&res.Status,
		&res.Port,
		&res.CreatedAt,
		&res.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entities.Container{}, fmt.Errorf("%s: %w", op, ErrNotFound)
		}

		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	return res, nil
}

func (c *Container) GetAll(ctx context.Context) ([]entities.Container, error) {
	const op = "storage.Container.GetAll"

	query, values, err := sq.Select(columns...).From(table).ToSql()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	rows, err := c.pool.Query(ctx, query, values...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var res []entities.Container

	for rows.Next() {
		var row entities.Container

		err = rows.Scan(
			&row.ID,
			&row.UserID,
			&row.ContainerID,
			&row.Status,
			&row.Port,
			&row.CreatedAt,
			&row.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}

		res = append(res, row)
	}

	return res, nil
}
