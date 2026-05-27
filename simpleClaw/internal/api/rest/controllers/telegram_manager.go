package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"shared/pkg/jwt"
	"shared/pkg/response"
	"simpleClaw/internal/api/rest/dto"
	"simpleClaw/internal/api/rest/middleware"
	"simpleClaw/internal/entities"
	infraSQL "simpleClaw/internal/infra/sql"
	telegraminfra "simpleClaw/internal/infra/telegram"
	telegrammanagerservice "simpleClaw/internal/service/telegrammanager"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type telegramManagerService interface {
	CreateLink(ctx context.Context, cmd telegrammanagerservice.CreateLinkCommand) (telegrammanagerservice.CreateLinkResult, error)
	GetManagedBot(ctx context.Context, id, userID uuid.UUID) (entities.TelegramManagedBot, error)
	HandleUpdate(ctx context.Context, update telegraminfra.Update) error
}

type TelegramManager struct {
	service       telegramManagerService
	j             jwt.JWT
	webhookSecret string
}

func NewTelegramManager(service telegramManagerService, j jwt.JWT, webhookSecret string) *TelegramManager {
	return &TelegramManager{
		service:       service,
		j:             j,
		webhookSecret: strings.TrimSpace(webhookSecret),
	}
}

func (t *TelegramManager) Register(r chi.Router) {
	r.With(middleware.AuthJwt(t.j)).Post("/me/telegram/manager/link", t.CreateLink)
	r.With(middleware.AuthJwt(t.j)).Get("/me/telegram/manager/link/{id}", t.GetLinkStatus)
	r.Post("/telegram/manager/webhook", t.HandleWebhook)
}

func (t *TelegramManager) CreateLink(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})
		return
	}

	var req dto.TelegramManagerCreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid request body",
		})
		return
	}

	clawID, err := uuid.Parse(strings.TrimSpace(req.ClawID))
	if err != nil {
		respondServiceError(w, infraSQL.ErrInvalid)
		return
	}

	result, err := t.service.CreateLink(r.Context(), telegrammanagerservice.CreateLinkCommand{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, dto.TelegramManagerCreateLinkResponse{
		ID:          result.ManagedBot.ID.String(),
		Status:      string(result.ManagedBot.Status),
		DeepLinkURL: result.DeepLinkURL,
	})
}

func (t *TelegramManager) GetLinkStatus(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})
		return
	}

	id, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "id")))
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid telegram manager id",
		})
		return
	}

	item, err := t.service.GetManagedBot(r.Context(), id, userID)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	res := dto.TelegramManagerStatusResponse{
		ID:                 item.ID.String(),
		Status:             string(item.Status),
		ManagedBotUserID:   item.ManagedBotUserID,
		ManagedBotName:     item.ManagedBotName,
		ManagedBotUsername: item.ManagedBotUsername,
	}
	if item.ChannelID != uuid.Nil {
		res.ChannelID = item.ChannelID.String()
	}

	response.RespondOK(w, res)
}

func (t *TelegramManager) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if t.webhookSecret == "" || r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != t.webhookSecret {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid webhook authorization",
		})
		return
	}

	var update telegraminfra.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid request body",
		})
		return
	}

	if err := t.service.HandleUpdate(r.Context(), update); err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, map[string]bool{"ok": true})
}
