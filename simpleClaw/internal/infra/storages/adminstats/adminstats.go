package adminstats

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"shared/pkg/observability"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Storage exposes read-only aggregate queries over the control-plane tables that
// back the admin dashboard and users listing. It never mutates state.
type Storage struct {
	db      *gorm.DB
	metrics *observability.OperationMetrics
}

func NewStorage(db *gorm.DB, metrics ...*observability.OperationMetrics) *Storage {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	return &Storage{db: db, metrics: opMetrics}
}

// clawIssuePredicate defines what makes a claw "in trouble": the runtime is in an
// error state or the last lifecycle transition failed. Kept in one place so the
// dashboard counter, the per-user flag and the listing filter stay consistent.
const clawIssuePredicate = "observed_state = ? OR lifecycle_status = ?"

func clawIssueArgs() []any {
	return []any{string(entities.ClawObservedStateError), string(entities.ClawLifecycleStatusFailed)}
}

// CountUsers returns the total number of users.
func (s *Storage) CountUsers(ctx context.Context) (int64, error) {
	const op = "storages.AdminStats.CountUsers"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.admin_stats", "storage.admin.count_users", "admin")

	var err error

	defer func() { finish(err) }()

	var count int64

	err = s.db.WithContext(ctx).Model(&models.User{}).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return count, nil
}

// CountUsersByRole returns the number of users carrying the given role.
func (s *Storage) CountUsersByRole(ctx context.Context, role string) (int64, error) {
	const op = "storages.AdminStats.CountUsersByRole"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.admin_stats", "storage.admin.count_users_by_role", "admin")

	var err error

	defer func() { finish(err) }()

	var count int64

	err = s.db.WithContext(ctx).Model(&models.User{}).Where("role = ?", role).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return count, nil
}

// CountUsersWithActiveSubscription returns the number of distinct users that have
// at least one active subscription.
func (s *Storage) CountUsersWithActiveSubscription(ctx context.Context) (int64, error) {
	const op = "storages.AdminStats.CountUsersWithActiveSubscription"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.admin_stats", "storage.admin.count_active_subscriptions", "admin")

	var err error

	defer func() { finish(err) }()

	var count int64

	err = s.db.WithContext(ctx).
		Model(&models.UserSubscription{}).
		Where("status = ?", entities.SubscriptionStatusActive).
		Distinct("user_id").
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return count, nil
}

// CountUsersWithIssues returns the number of distinct users owning at least one
// claw in an error/failed state.
func (s *Storage) CountUsersWithIssues(ctx context.Context) (int64, error) {
	const op = "storages.AdminStats.CountUsersWithIssues"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.admin_stats", "storage.admin.count_users_with_issues", "admin")

	var err error

	defer func() { finish(err) }()

	var count int64

	err = s.db.WithContext(ctx).
		Model(&models.Claw{}).
		Where(clawIssuePredicate, clawIssueArgs()...).
		Distinct("user_id").
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return count, nil
}

type userRow struct {
	ID                    uuid.UUID
	Email                 string
	Name                  string
	NickName              string
	AvatarURL             string
	Role                  string
	BalanceMinor          int64
	ClawsCount            int64
	HasActiveSubscription bool
	HasIssues             bool
	CreatedAt             time.Time
}

// ListUsers returns a filtered, sorted, paginated page of users together with the
// total number of rows matching the filter (ignoring pagination). Computed
// columns are resolved with correlated subqueries so a single round trip yields
// claws_count, has_active_subscription and has_issues.
func (s *Storage) ListUsers(
	ctx context.Context,
	filter entities.AdminUserFilter,
) ([]entities.AdminUserListItem, int64, error) {
	const op = "storages.AdminStats.ListUsers"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.admin_stats", "storage.admin.list_users", "admin")

	var err error

	defer func() { finish(err) }()

	var total int64

	err = s.filtered(ctx, filter).Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	rows := make([]userRow, 0, filter.PageSize)

	err = s.filtered(ctx, filter).
		Select(userSelectColumns, entities.SubscriptionStatusActive, string(entities.ClawObservedStateError), string(entities.ClawLifecycleStatusFailed)).
		Order(orderClause(filter.Sort)).
		Limit(filter.PageSize).
		Offset((filter.Page - 1) * filter.PageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	items := make([]entities.AdminUserListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, entities.AdminUserListItem{
			ID:                    row.ID,
			Email:                 row.Email,
			Name:                  row.Name,
			Nickname:              row.NickName,
			AvatarURL:             row.AvatarURL,
			Role:                  row.Role,
			BalanceMinor:          row.BalanceMinor,
			HasActiveSubscription: row.HasActiveSubscription,
			ClawsCount:            row.ClawsCount,
			HasIssues:             row.HasIssues,
			CreatedAt:             row.CreatedAt,
		})
	}

	return items, total, nil
}

const userSelectColumns = `u.id, u.email, u.name, u.nick_name, u.avatar_url, u.role, u.balance_minor, u.created_at,
	(SELECT COUNT(*) FROM claws c WHERE c.user_id = u.id) AS claws_count,
	EXISTS (SELECT 1 FROM user_subscriptions su WHERE su.user_id = u.id AND su.status = ?) AS has_active_subscription,
	EXISTS (SELECT 1 FROM claws c WHERE c.user_id = u.id AND (c.observed_state = ? OR c.lifecycle_status = ?)) AS has_issues`

// filtered builds the base query (table + WHERE) shared by the count and the page
// query. It is called once per query so each caller gets an independent chain.
func (s *Storage) filtered(ctx context.Context, filter entities.AdminUserFilter) *gorm.DB {
	db := s.db.WithContext(ctx).Table("users AS u")

	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		db = db.Where(
			"LOWER(u.email) LIKE ? OR LOWER(u.name) LIKE ? OR LOWER(u.nick_name) LIKE ?",
			like, like, like,
		)
	}

	if role := strings.TrimSpace(filter.Role); role != "" {
		db = db.Where("u.role = ?", role)
	}

	if filter.RegisteredFrom != nil {
		db = db.Where("u.created_at >= ?", *filter.RegisteredFrom)
	}

	if filter.RegisteredTo != nil {
		db = db.Where("u.created_at <= ?", *filter.RegisteredTo)
	}

	if filter.HasActiveSubscription != nil {
		exists := "EXISTS (SELECT 1 FROM user_subscriptions su WHERE su.user_id = u.id AND su.status = ?)"
		if *filter.HasActiveSubscription {
			db = db.Where(exists, entities.SubscriptionStatusActive)
		} else {
			db = db.Where("NOT "+exists, entities.SubscriptionStatusActive)
		}
	}

	if filter.HasIssues != nil {
		exists := "EXISTS (SELECT 1 FROM claws c WHERE c.user_id = u.id AND (c.observed_state = ? OR c.lifecycle_status = ?))"
		if *filter.HasIssues {
			db = db.Where(exists, string(entities.ClawObservedStateError), string(entities.ClawLifecycleStatusFailed))
		} else {
			db = db.Where("NOT "+exists, string(entities.ClawObservedStateError), string(entities.ClawLifecycleStatusFailed))
		}
	}

	return db
}

// orderClause maps the whitelisted sort enum to a safe ORDER BY fragment. Any
// unknown value falls back to newest-first.
func orderClause(sort entities.AdminUserSort) string {
	switch sort {
	case entities.AdminUserSortCreatedAtAsc:
		return "u.created_at ASC"
	case entities.AdminUserSortBalanceDesc:
		return "u.balance_minor DESC"
	case entities.AdminUserSortBalanceAsc:
		return "u.balance_minor ASC"
	case entities.AdminUserSortEmailAsc:
		return "u.email ASC"
	default:
		return "u.created_at DESC"
	}
}
