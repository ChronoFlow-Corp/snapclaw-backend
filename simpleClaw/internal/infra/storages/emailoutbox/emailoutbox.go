package emailoutbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxStoredErrorLen = 1000

type Storage struct {
	db *gorm.DB
}

func NewStorage(db *gorm.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) Enqueue(ctx context.Context, m entities.OutboxEmail) error {
	const op = "storages.EmailOutbox.Enqueue"

	headers, err := json.Marshal(m.Headers)
	if err != nil {
		return fmt.Errorf("%s: marshal headers: %w", op, err)
	}

	status := m.Status
	if status == "" {
		status = entities.EmailStatusPending
	}

	row := models.EmailOutbox{
		ID:            m.ID,
		ToAddress:     m.ToAddress,
		Subject:       m.Subject,
		HTMLBody:      m.HTMLBody,
		TextBody:      m.TextBody,
		Headers:       string(headers),
		Category:      string(m.Category),
		Status:        string(status),
		Attempts:      m.Attempts,
		MaxAttempts:   m.MaxAttempts,
		NextAttemptAt: m.NextAttemptAt,
	}

	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

// ClaimDue returns pending emails whose next attempt time has arrived.
func (s *Storage) ClaimDue(ctx context.Context, now time.Time, limit int) ([]entities.OutboxEmail, error) {
	const op = "storages.EmailOutbox.ClaimDue"

	if limit <= 0 {
		limit = 20
	}

	var rows []models.EmailOutbox

	err := s.db.WithContext(ctx).
		Where("status = ? AND next_attempt_at <= ?", string(entities.EmailStatusPending), now).
		Order("created_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	out := make([]entities.OutboxEmail, 0, len(rows))
	for _, row := range rows {
		out = append(out, toEntity(row))
	}

	return out, nil
}

func (s *Storage) MarkSent(ctx context.Context, id uuid.UUID, providerMessageID string, sentAt time.Time) error {
	const op = "storages.EmailOutbox.MarkSent"

	tx := s.db.WithContext(ctx).Model(&models.EmailOutbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":              string(entities.EmailStatusSent),
			"provider_message_id": providerMessageID,
			"sent_at":             sentAt,
			"last_error":          "",
		})
	if tx.Error != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(tx.Error))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) MarkFailed(
	ctx context.Context,
	id uuid.UUID,
	attempts int,
	lastErr string,
	nextAttemptAt time.Time,
	status entities.EmailStatus,
) error {
	const op = "storages.EmailOutbox.MarkFailed"

	if len(lastErr) > maxStoredErrorLen {
		lastErr = lastErr[:maxStoredErrorLen]
	}

	tx := s.db.WithContext(ctx).Model(&models.EmailOutbox{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":          string(status),
			"attempts":        attempts,
			"last_error":      lastErr,
			"next_attempt_at": nextAttemptAt,
		})
	if tx.Error != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(tx.Error))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func toEntity(row models.EmailOutbox) entities.OutboxEmail {
	var headers map[string]string
	if strings.TrimSpace(row.Headers) != "" {
		_ = json.Unmarshal([]byte(row.Headers), &headers)
	}

	return entities.OutboxEmail{
		ID:                row.ID,
		ToAddress:         row.ToAddress,
		Subject:           row.Subject,
		HTMLBody:          row.HTMLBody,
		TextBody:          row.TextBody,
		Headers:           headers,
		Category:          entities.EmailCategory(row.Category),
		Status:            entities.EmailStatus(row.Status),
		Attempts:          row.Attempts,
		MaxAttempts:       row.MaxAttempts,
		LastError:         row.LastError,
		ProviderMessageID: row.ProviderMessageID,
		NextAttemptAt:     row.NextAttemptAt,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		SentAt:            row.SentAt,
	}
}
