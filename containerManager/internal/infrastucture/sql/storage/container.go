package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"containermanager/internal/entities"
	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"shared/pkg/observability"
)

const (
	table = "containers"
)

const (
	id = iota
	userID
	clawID
	containerID
	status
	port
	hasStartedOnce
	createdAt
	updatedAt
)

var columns = []string{
	id:             "id",
	userID:         "user_id",
	clawID:         "claw_id",
	containerID:    "container_id",
	status:         "status",
	port:           "port",
	hasStartedOnce: "has_started_once",
	createdAt:      "created_at",
	updatedAt:      "updated_at",
}

type Container struct {
	pool    *pgxpool.Pool
	metrics *observability.OperationMetrics
}

var sq = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

func NewContainer(pool *pgxpool.Pool, metrics ...*observability.OperationMetrics) *Container {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	return &Container{pool: pool, metrics: opMetrics}
}

func (c *Container) Create(ctx context.Context, cl entities.Container) (err error) {
	const op = "storage.Container.Create"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), c.metrics, "storage.containers", "storage.container.create", "claw_lifecycle")

	defer func() { finish(err) }()

	query, values, err := sq.Insert(table).
		Columns(columns[id], columns[containerID], columns[port], columns[userID], columns[clawID], columns[hasStartedOnce]).
		Values(cl.ID, cl.ContainerID, cl.Port, cl.UserID, cl.ClawID, cl.HasStartedOnce).
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

func (c *Container) Update(ctx context.Context, cl entities.Container) (err error) {
	const op = "storage.Container.Update"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), c.metrics, "storage.containers", "storage.container.update", "claw_lifecycle")

	defer func() { finish(err) }()

	cDb, err := c.GetByID(ctx, cl.ID.String())
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

	if cDb.HasStartedOnce != cl.HasStartedOnce {
		b = b.Set(columns[hasStartedOnce], cl.HasStartedOnce)
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

func (c *Container) Remove(ctx context.Context, cl entities.Container) (err error) {
	const op = "storage.Container.Remove"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), c.metrics, "storage.containers", "storage.container.remove", "claw_lifecycle")

	defer func() { finish(err) }()

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

func (c *Container) GetByUserClawID(ctx context.Context, uID, cID string) (entities.Container, error) {
	const op = "storage.Container.GetByUserClawID"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), c.metrics, "storage.containers", "storage.container.get_by_user_claw_id", "claw_lifecycle")

	var err error

	defer func() { finish(err) }()

	query, values, err := sq.Select(columns...).
		From(table).
		Where(squirrel.Eq{
			columns[userID]: uID,
			columns[clawID]: cID,
		}).
		ToSql()
	if err != nil {
		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	var res entities.Container

	row := c.pool.QueryRow(ctx, query, values...)

	err = row.Scan(
		&res.ID,
		&res.UserID,
		&res.ClawID,
		&res.ContainerID,
		&res.Status,
		&res.Port,
		&res.HasStartedOnce,
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

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), c.metrics, "storage.containers", "storage.container.get_by_id", "claw_lifecycle")

	var err error

	defer func() { finish(err) }()

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
		&res.ClawID,
		&res.ContainerID,
		&res.Status,
		&res.Port,
		&res.HasStartedOnce,
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

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), c.metrics, "storage.containers", "storage.container.list", "claw_lifecycle")

	var err error

	defer func() { finish(err) }()

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
			&row.ClawID,
			&row.ContainerID,
			&row.Status,
			&row.Port,
			&row.HasStartedOnce,
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
