package balanceentries

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"simpleClaw/internal/infra/storages/payments"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"
)

var ErrInsufficientBalance = errors.New("insufficient balance")

type Storage struct {
	db *gorm.DB
}

func NewStorage(db *gorm.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) ListByUserID(
	ctx context.Context,
	userID uuid.UUID,
	limit int,
) ([]entities.UserBalanceEntry, error) {
	const op = "storages.BalanceEntries.ListByUserID"

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	query := s.db.WithContext(ctx).
		Model(&models.UserBalanceEntry{}).
		Where("user_id = ?", userID).
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var rows []models.UserBalanceEntry
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	entries := make([]entities.UserBalanceEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, mapToEntity(row))
	}

	return entries, nil
}

func (s *Storage) ApplyCredit(
	ctx context.Context,
	entry entities.UserBalanceEntry,
	payment entities.Payment,
) (int64, error) {
	const op = "storages.BalanceEntries.ApplyCredit"

	if err := validateEntry(entry); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	balance, err := s.apply(ctx, entry, &payment, false)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return balance, nil
}

func (s *Storage) ApplyUsageDebit(
	ctx context.Context,
	entry entities.UserBalanceEntry,
) (int64, error) {
	const op = "storages.BalanceEntries.ApplyUsageDebit"

	if err := validateEntry(entry); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	balance, err := s.apply(ctx, entry, nil, true)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return balance, nil
}

func (s *Storage) GetByPaymentID(
	ctx context.Context,
	paymentID string,
) (entities.UserBalanceEntry, error) {
	const op = "storages.BalanceEntries.GetByPaymentID"

	b, err := gorm.G[models.UserBalanceEntry](s.db).Where("payment_id = ?", paymentID).First(ctx)
	if err != nil {
		return entities.UserBalanceEntry{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return mapToEntity(b), nil
}

func (s *Storage) ApplyUsageDebitOnce(
	ctx context.Context,
	entry entities.UserBalanceEntry,
	event entities.OpenRouterUsageEvent,
) (int64, bool, error) {
	const op = "storages.BalanceEntries.ApplyUsageDebitOnce"

	if err := validateEntry(entry); err != nil {
		return 0, false, fmt.Errorf("%s: %w", op, err)
	}

	if err := validateUsageEvent(event); err != nil {
		return 0, false, fmt.Errorf("%s: %w", op, err)
	}

	var (
		newBalance int64
		applied    bool
	)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", entry.UserID).Error; err != nil {
			return sql.TranslateError(err)
		}

		eventModel := models.OpenRouterUsageEvent{
			ID:         event.ID,
			Provider:   event.Provider,
			TraceID:    event.TraceID,
			SpanID:     event.SpanID,
			UserID:     event.UserID,
			Model:      event.Model,
			APIKeyName: event.APIKeyName,
			CreatedAt:  event.CreatedAt,
		}

		if err := tx.Create(&eventModel).Error; err != nil {
			if isDuplicateUsageEventErr(err) {
				newBalance = user.BalanceMinor
				applied = false
				return nil
			}
			return sql.TranslateError(err)
		}

		if user.BalanceMinor < entry.AmountMinor {
			return ErrInsufficientBalance
		}

		newBalance = user.BalanceMinor - entry.AmountMinor
		applied = true

		model := mapToModel(entry)
		if err := tx.Create(&model).Error; err != nil {
			return sql.TranslateError(err)
		}

		updateTx := tx.Model(&models.User{}).
			Where("id = ?", user.ID).
			Update("balance_minor", newBalance)
		if updateTx.Error != nil {
			return sql.TranslateError(updateTx.Error)
		}

		if updateTx.RowsAffected == 0 {
			return sql.ErrNotFound
		}

		return nil
	})
	if err != nil {
		return 0, false, fmt.Errorf("%s: %w", op, err)
	}

	return newBalance, applied, nil
}

func (s *Storage) apply(
	ctx context.Context,
	entry entities.UserBalanceEntry,
	payment *entities.Payment,
	debit bool,
) (int64, error) {
	var newBalance int64

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", entry.UserID).Error; err != nil {
			return sql.TranslateError(err)
		}

		if debit {
			if user.BalanceMinor < entry.AmountMinor {
				return ErrInsufficientBalance
			}

			newBalance = user.BalanceMinor - entry.AmountMinor
		} else {
			newBalance = user.BalanceMinor + entry.AmountMinor
		}

		model := mapToModel(entry)
		if err := tx.Create(&model).Error; err != nil {
			return sql.TranslateError(err)
		}

		updateTx := tx.Model(&models.User{}).
			Where("id = ?", user.ID).
			Update("balance_minor", newBalance)
		if updateTx.Error != nil {
			return sql.TranslateError(updateTx.Error)
		}

		if updateTx.RowsAffected == 0 {
			return sql.ErrNotFound
		}

		if payment != nil && payment.ID != "" {
			pDB, err := payments.MapPaymentToModel(*payment)
			if err != nil {
				return fmt.Errorf("map payment snapshot %s: %w", payment.ID, err)
			}
			_, err = gorm.G[models.Payment](tx).Where("id = ?", payment.ID).Updates(ctx, pDB)
			if err != nil {
				return fmt.Errorf(
					"update payment snapshot %s: %w",
					payment.ID,
					sql.TranslateError(err),
				)
			}
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	return newBalance, nil
}

func validateEntry(entry entities.UserBalanceEntry) error {
	switch {
	case entry.ID == uuid.Nil:
		return invalidEntry("entry id is required")
	case entry.UserID == uuid.Nil:
		return invalidEntry("entry user_id is required")
	case entry.Type == "":
		return invalidEntry("entry type is required")
	case entry.AmountMinor <= 0:
		return invalidEntry("entry amount_minor must be greater than 0")
	case entry.CreatedAt.IsZero():
		return invalidEntry("entry created_at is required")
	default:
		return nil
	}
}

func validateUsageEvent(event entities.OpenRouterUsageEvent) error {
	switch {
	case event.ID == uuid.Nil:
		return invalidUsageEvent("usage event id is required")
	case event.Provider == "":
		return invalidUsageEvent("usage event provider is required")
	case event.TraceID == "":
		return invalidUsageEvent("usage event trace_id is required")
	case event.SpanID == "":
		return invalidUsageEvent("usage event span_id is required")
	case event.UserID == uuid.Nil:
		return invalidUsageEvent("usage event user_id is required")
	case event.APIKeyName == "":
		return invalidUsageEvent("usage event api_key_name is required")
	case event.CreatedAt.IsZero():
		return invalidUsageEvent("usage event created_at is required")
	default:
		return nil
	}
}

func invalidEntry(reason string) error {
	return fmt.Errorf("%s: %w", reason, sql.ErrInvalid)
}

func invalidUsageEvent(reason string) error {
	return fmt.Errorf("%s: %w", reason, sql.ErrInvalid)
}

func isDuplicateUsageEventErr(err error) bool {
	translated := sql.TranslateError(err)
	if errors.Is(translated, sql.ErrConflict) {
		return true
	}

	return strings.Contains(strings.ToLower(err.Error()), "unique")
}

func mapToModel(entry entities.UserBalanceEntry) models.UserBalanceEntry {
	return models.UserBalanceEntry{
		ID:             entry.ID,
		UserID:         entry.UserID,
		Type:           entry.Type,
		AmountMinor:    entry.AmountMinor,
		PaymentID:      entry.PaymentID,
		SubscriptionID: entry.SubscriptionID,
		Description:    entry.Description,
		CreatedAt:      entry.CreatedAt,
	}
}

func mapToEntity(entry models.UserBalanceEntry) entities.UserBalanceEntry {
	return entities.UserBalanceEntry{
		ID:             entry.ID,
		UserID:         entry.UserID,
		Type:           entry.Type,
		AmountMinor:    entry.AmountMinor,
		PaymentID:      entry.PaymentID,
		SubscriptionID: entry.SubscriptionID,
		Description:    entry.Description,
		CreatedAt:      entry.CreatedAt,
	}
}
