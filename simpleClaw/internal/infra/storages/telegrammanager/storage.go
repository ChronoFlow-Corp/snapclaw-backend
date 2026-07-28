package telegrammanager

import (
	"context"
	"fmt"
	"strings"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Storage struct {
	db *gorm.DB
}

func NewStorage(db *gorm.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) CreateLink(ctx context.Context, link entities.TelegramAccountLink) error {
	const op = "storages.TelegramManager.CreateLink"

	if link.ID == uuid.Nil || link.UserID == uuid.Nil || strings.TrimSpace(link.LinkCodeHash) == "" {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	err := s.db.WithContext(ctx).Create(&models.TelegramAccountLink{
		ID:               link.ID,
		UserID:           link.UserID,
		LinkCodeHash:     strings.TrimSpace(link.LinkCodeHash),
		TelegramUserID:   link.TelegramUserID,
		TelegramChatID:   link.TelegramChatID,
		TelegramUsername: strings.TrimSpace(link.TelegramUsername),
		Status:           string(link.Status),
		ExpiresAt:        link.ExpiresAt,
		CreatedAt:        link.CreatedAt,
		UpdatedAt:        link.UpdatedAt,
	}).Error
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) ConsumeLink(
	ctx context.Context,
	linkCodeHash string,
	activation entities.TelegramLinkActivation,
) (entities.TelegramAccountLink, error) {
	const op = "storages.TelegramManager.ConsumeLink"

	if strings.TrimSpace(linkCodeHash) == "" || activation.TelegramUserID == 0 || activation.LinkedAt.IsZero() {
		return entities.TelegramAccountLink{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	var row models.TelegramAccountLink

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("link_code_hash = ? AND status = ? AND expires_at > ?", strings.TrimSpace(linkCodeHash), string(entities.TelegramAccountLinkStatusPending), activation.LinkedAt).
			First(&row).Error; err != nil {
			return sql.TranslateError(err)
		}

		row.TelegramUserID = activation.TelegramUserID
		row.TelegramChatID = activation.TelegramChatID
		row.TelegramUsername = strings.TrimSpace(activation.TelegramUsername)
		row.Status = string(entities.TelegramAccountLinkStatusLinked)
		row.UpdatedAt = activation.LinkedAt

		if err := tx.Save(&row).Error; err != nil {
			return sql.TranslateError(err)
		}

		return nil
	})
	if err != nil {
		return entities.TelegramAccountLink{}, fmt.Errorf("%s: %w", op, err)
	}

	return mapAccountLink(row), nil
}

func (s *Storage) GetByTelegramUserID(ctx context.Context, telegramUserID int64) (entities.TelegramAccountLink, error) {
	const op = "storages.TelegramManager.GetByTelegramUserID"

	if telegramUserID == 0 {
		return entities.TelegramAccountLink{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	var row models.TelegramAccountLink
	if err := s.db.WithContext(ctx).
		Where("telegram_user_id = ?", telegramUserID).
		First(&row).Error; err != nil {
		return entities.TelegramAccountLink{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return mapAccountLink(row), nil
}

func (s *Storage) TryRecordWebhookUpdate(ctx context.Context, updateID int64) (bool, error) {
	const op = "storages.TelegramManager.TryRecordWebhookUpdate"

	if updateID == 0 {
		return false, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&models.TelegramWebhookUpdate{
		UpdateID: updateID,
	})
	if result.Error != nil {
		return false, fmt.Errorf("%s: %w", op, sql.TranslateError(result.Error))
	}

	return result.RowsAffected > 0, nil
}

func (s *Storage) UpsertManagedBot(ctx context.Context, bot entities.TelegramManagedBot) error {
	const op = "storages.TelegramManager.UpsertManagedBot"

	if bot.ID == uuid.Nil || bot.UserID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	model := models.TelegramManagedBot{
		ID:                 bot.ID,
		UserID:             bot.UserID,
		ManagedBotUsername: strings.TrimSpace(bot.ManagedBotUsername),
		ManagedBotName:     strings.TrimSpace(bot.ManagedBotName),
		DeepLinkURL:        strings.TrimSpace(bot.DeepLinkURL),
		Status:             string(bot.Status),
		LastError:          strings.TrimSpace(bot.LastError),
		CreatedAt:          bot.CreatedAt,
		UpdatedAt:          bot.UpdatedAt,
	}
	if bot.ClawID != uuid.Nil {
		clawID := bot.ClawID
		model.ClawID = &clawID
	}
	if bot.TelegramOwnerUserID != 0 {
		telegramOwnerUserID := bot.TelegramOwnerUserID
		model.TelegramOwnerUserID = &telegramOwnerUserID
	}
	if bot.ManagedBotUserID != 0 {
		managedBotUserID := bot.ManagedBotUserID
		model.ManagedBotUserID = &managedBotUserID
	}
	if !bot.LinkExpiresAt.IsZero() {
		linkExpiresAt := bot.LinkExpiresAt
		model.LinkExpiresAt = &linkExpiresAt
	}
	if bot.ChannelID != uuid.Nil {
		channelID := bot.ChannelID
		model.ChannelID = &channelID
	}

	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"user_id":                model.UserID,
			"claw_id":                model.ClawID,
			"telegram_owner_user_id": model.TelegramOwnerUserID,
			"managed_bot_user_id":    model.ManagedBotUserID,
			"managed_bot_username":   model.ManagedBotUsername,
			"managed_bot_name":       model.ManagedBotName,
			"deep_link_url":          model.DeepLinkURL,
			"link_expires_at":        model.LinkExpiresAt,
			"channel_id":             model.ChannelID,
			"status":                 model.Status,
			"last_error":             model.LastError,
			"updated_at":             model.UpdatedAt,
		}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) GetManagedBotByID(
	ctx context.Context,
	id uuid.UUID,
	userID uuid.UUID,
) (entities.TelegramManagedBot, error) {
	const op = "storages.TelegramManager.GetManagedBotByID"

	if id == uuid.Nil || userID == uuid.Nil {
		return entities.TelegramManagedBot{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	var row models.TelegramManagedBot
	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&row).Error; err != nil {
		return entities.TelegramManagedBot{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return mapManagedBot(row), nil
}

func (s *Storage) GetLatestManagedBotByTelegramOwnerUserID(
	ctx context.Context,
	telegramOwnerUserID int64,
) (entities.TelegramManagedBot, error) {
	const op = "storages.TelegramManager.GetLatestManagedBotByTelegramOwnerUserID"

	if telegramOwnerUserID == 0 {
		return entities.TelegramManagedBot{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	var row models.TelegramManagedBot
	if err := s.db.WithContext(ctx).
		Where("telegram_owner_user_id = ?", telegramOwnerUserID).
		Order("updated_at DESC").
		First(&row).Error; err != nil {
		return entities.TelegramManagedBot{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return mapManagedBot(row), nil
}

func (s *Storage) GetLatestManagedBotByClawID(
	ctx context.Context,
	userID uuid.UUID,
	clawID uuid.UUID,
) (entities.TelegramManagedBot, error) {
	const op = "storages.TelegramManager.GetLatestManagedBotByClawID"

	if userID == uuid.Nil || clawID == uuid.Nil {
		return entities.TelegramManagedBot{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	var row models.TelegramManagedBot
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND claw_id = ?", userID, clawID).
		Order("updated_at DESC").
		First(&row).Error; err != nil {
		return entities.TelegramManagedBot{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return mapManagedBot(row), nil
}

func (s *Storage) ListManagedBotsByUserID(
	ctx context.Context,
	userID uuid.UUID,
) ([]entities.TelegramManagedBot, error) {
	const op = "storages.TelegramManager.ListManagedBotsByUserID"

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	var rows []models.TelegramManagedBot
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	out := make([]entities.TelegramManagedBot, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapManagedBot(row))
	}

	return out, nil
}

func mapAccountLink(row models.TelegramAccountLink) entities.TelegramAccountLink {
	return entities.TelegramAccountLink{
		ID:               row.ID,
		UserID:           row.UserID,
		LinkCodeHash:     row.LinkCodeHash,
		TelegramUserID:   row.TelegramUserID,
		TelegramChatID:   row.TelegramChatID,
		TelegramUsername: row.TelegramUsername,
		Status:           entities.TelegramAccountLinkStatus(row.Status),
		ExpiresAt:        row.ExpiresAt,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func mapManagedBot(row models.TelegramManagedBot) entities.TelegramManagedBot {
	out := entities.TelegramManagedBot{
		ID:                 row.ID,
		UserID:             row.UserID,
		ManagedBotUsername: row.ManagedBotUsername,
		ManagedBotName:     row.ManagedBotName,
		DeepLinkURL:        row.DeepLinkURL,
		Status:             entities.TelegramManagedBotStatus(row.Status),
		LastError:          row.LastError,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
	if row.ClawID != nil {
		out.ClawID = *row.ClawID
	}
	if row.TelegramOwnerUserID != nil {
		out.TelegramOwnerUserID = *row.TelegramOwnerUserID
	}
	if row.ManagedBotUserID != nil {
		out.ManagedBotUserID = *row.ManagedBotUserID
	}
	if row.LinkExpiresAt != nil {
		out.LinkExpiresAt = *row.LinkExpiresAt
	}
	if row.ChannelID != nil {
		out.ChannelID = *row.ChannelID
	}

	return out
}
