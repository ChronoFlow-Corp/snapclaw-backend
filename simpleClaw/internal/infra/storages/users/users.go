package users

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"shared/pkg/observability"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"
)

type Storage struct {
	db      *gorm.DB
	metrics *observability.OperationMetrics
}

func NewStorage(db *gorm.DB, metrics ...*observability.OperationMetrics) *Storage {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	return &Storage{
		db:      db,
		metrics: opMetrics,
	}
}

func (s *Storage) Create(ctx context.Context, u entities.User) error {
	const op = "storages.Users.Create"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.users", "storage.user.create", "user_profile")

	var err error

	defer func() { finish(err) }()

	err = gorm.G[models.User](s.db).Create(ctx, &models.User{
		ID:               u.ID,
		Name:             u.Name,
		NickName:         u.Nickname,
		AvatarURL:        u.AvatarURL,
		Email:            u.Email,
		Role:             u.Role,
		OpenRouterApiKey: u.OpenRouterApiKey,
		OpenRouterKeyID:  u.OpenRouterKeyID,
		BalanceMinor:     u.BalanceMinor,
		CreatedAt:        u.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) Delete(ctx context.Context, id uuid.UUID) (err error) {
	const op = "storages.Users.Delete"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.users", "storage.user.delete", "user_profile")

	defer func() { finish(err) }()

	affected, err := gorm.G[models.User](s.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if affected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) CreateSession(ctx context.Context, session entities.Session) (err error) {
	const op = "storages.Users.CreateSession"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.users", "storage.session.create", "auth")

	defer func() { finish(err) }()

	err = gorm.G[models.Session](s.db).Create(ctx, &models.Session{
		ID:           session.ID,
		UserID:       session.UserID,
		RefreshToken: session.RefreshToken,
		CreatedAt:    session.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) GetByEmail(ctx context.Context, email string) (entities.User, error) {
	const op = "storages.Users.GetByEmail"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.users", "storage.user.get_by_email", "user_profile")

	var err error

	defer func() { finish(err) }()

	email = strings.ToLower(strings.TrimSpace(email))

	uDB, err := gorm.G[models.User](s.db).Where("LOWER(email) = ?", email).First(ctx)
	if err != nil {
		return entities.User{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return entities.User{
		ID:               uDB.ID,
		Name:             uDB.Name,
		Nickname:         uDB.NickName,
		AvatarURL:        uDB.AvatarURL,
		Email:            uDB.Email,
		Role:             uDB.Role,
		OpenRouterApiKey: uDB.OpenRouterApiKey,
		OpenRouterKeyID:  uDB.OpenRouterKeyID,
		BalanceMinor:     uDB.BalanceMinor,
		CreatedAt:        uDB.CreatedAt,
	}, nil
}

func (s *Storage) UpdateRole(ctx context.Context, id uuid.UUID, role string) (err error) {
	const op = "storages.Users.UpdateRole"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.users", "storage.user.update_role", "user_profile")

	defer func() { finish(err) }()

	tx := s.db.WithContext(ctx).Model(&models.User{}).
		Where("id = ?", id).
		Update("role", role)
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) GetByID(ctx context.Context, id uuid.UUID) (entities.User, error) {
	const op = "storages.Users.GetByID"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.users", "storage.user.get_by_id", "user_profile")

	var err error

	defer func() { finish(err) }()

	uDB, err := gorm.G[models.User](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return entities.User{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return entities.User{
		ID:               uDB.ID,
		Name:             uDB.Name,
		Email:            uDB.Email,
		Nickname:         uDB.NickName,
		AvatarURL:        uDB.AvatarURL,
		Role:             uDB.Role,
		OpenRouterApiKey: uDB.OpenRouterApiKey,
		OpenRouterKeyID:  uDB.OpenRouterKeyID,
		BalanceMinor:     uDB.BalanceMinor,
		CreatedAt:        uDB.CreatedAt,
	}, nil
}

func (s *Storage) DeleteSession(ctx context.Context, session entities.Session) (err error) {
	const op = "storages.Users.DeleteSession"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.users", "storage.session.delete", "auth")

	defer func() { finish(err) }()

	_, err = gorm.G[models.Session](
		s.db,
	).Where("id = ? AND user_id = ?", session.ID, session.UserID).
		Delete(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) GetSessions(ctx context.Context, userID uuid.UUID) ([]entities.Session, error) {
	const op = "storages.Users.GetSessions"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.users", "storage.session.list", "auth")

	var err error

	defer func() { finish(err) }()

	sessions := make([]entities.Session, 0)

	sDB, err := gorm.G[models.Session](s.db).Where("user_id = ?", userID).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	for _, s := range sDB {
		sessions = append(sessions, entities.Session{
			ID:           s.ID,
			UserID:       s.UserID,
			RefreshToken: s.RefreshToken,
			CreatedAt:    s.CreatedAt,
		})
	}

	return sessions, nil
}

func (s *Storage) GetSession(ctx context.Context, id uuid.UUID) (entities.Session, error) {
	const op = "storages.Users.GetSession"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.users", "storage.session.get", "auth")

	var err error

	defer func() { finish(err) }()

	sDB, err := gorm.G[models.Session](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return entities.Session{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return entities.Session{
		ID:           sDB.ID,
		UserID:       sDB.UserID,
		RefreshToken: sDB.RefreshToken,
		CreatedAt:    sDB.CreatedAt,
	}, nil
}

func (s *Storage) UpdateSessionRefresh(
	ctx context.Context,
	sessionID, userID uuid.UUID,
	refreshToken string,
) (err error) {
	const op = "storages.Users.UpdateSessionRefresh"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.users", "storage.session.update_refresh", "auth")

	defer func() { finish(err) }()

	updates := map[string]any{
		"refresh_token": refreshToken,
	}

	tx := s.db.WithContext(ctx).Model(&models.Session{}).
		Where("id = ? AND user_id = ?", sessionID, userID).
		Updates(updates)
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) UpdateOpenRouterKey(
	ctx context.Context,
	id uuid.UUID,
	key entities.OpenRouterKey,
) (err error) {
	const op = "storages.Users.UpdateOpenRouterKey"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.users", "storage.user.update_openrouter_key", "user_profile")

	defer func() { finish(err) }()

	updates := map[string]any{
		"open_router_api_key": key.Secret,
		"open_router_key_id":  key.ID,
	}

	tx := s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Updates(updates)
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) GetGmailToken(ctx context.Context, userID uuid.UUID) (entities.GmailToken, error) {
	const op = "storages.Users.GetGmailToken"

	tDB, err := gorm.G[models.GmailToken](s.db).Where("user_id = ?", userID).First(ctx)
	if err != nil {
		return entities.GmailToken{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	expiry := ""

	if !tDB.ExpiresAt.IsZero() {
		expiry = tDB.ExpiresAt.UTC().Format(time.RFC3339)
	}

	return entities.GmailToken{
		Email:  tDB.Email,
		Client: tDB.Client,
		Token: entities.Token{
			AccessToken:  tDB.AccessToken,
			RefreshToken: tDB.RefreshToken,
			TokenType:    tDB.TokenType,
			Expiry:       expiry,
		},
	}, nil
}

func (s *Storage) UpsertGmailToken(ctx context.Context, userID uuid.UUID, token entities.GmailToken) error {
	const op = "storages.Users.UpsertGmailToken"

	expiresAt, err := parseExpiry(token.Token.Expiry)
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	model := models.GmailToken{
		UserID:       userID,
		Email:        token.Email,
		Client:       token.Client,
		AccessToken:  token.Token.AccessToken,
		RefreshToken: token.Token.RefreshToken,
		TokenType:    token.Token.TokenType,
		ExpiresAt:    expiresAt,
	}

	tx := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"access_token",
			"refresh_token",
			"expires_at",
			"updated_at",
		}),
	}).Create(&model)
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func parseExpiry(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}

	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, nil
	}

	return time.Parse(time.RFC3339, value)
}
