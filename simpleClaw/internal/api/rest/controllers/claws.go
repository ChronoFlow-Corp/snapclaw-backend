package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"shared/pkg/response"

	"simpleClaw/internal/api/rest/dto"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/service/claw/commands"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type clawService interface {
	Create(ctx context.Context, cm commands.CreateClaw) (entities.Claw, error)
}

type Claw struct {
	service clawService
}

func NewClaw(service clawService) *Claw {
	return &Claw{
		service: service,
	}
}

func (c *Claw) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		// r.Use(middleware.AuthJwt(...))
		r.Post("/claws", c.Create)
	})
}

func (c *Claw) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateClawRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})

		return
	}

	userID, ok := r.Context().Value(entities.UserIDCtxKey{}).(uuid.UUID)
	if !ok {
		userID, _ = uuid.Parse("0de1813b-cc5b-4d85-93bb-0960841fcd93")
	}

	channelIDs := make([]uuid.UUID, 0, len(req.ChannelIDs))
	for _, raw := range req.ChannelIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.RespondError(w, response.Error{
				Code:    http.StatusBadRequest,
				Message: "invalid channel id: " + raw,
			})

			return
		}

		channelIDs = append(channelIDs, id)
	}

	var limits commands.ApiKeyLimits
	if req.ApiLimits != nil {
		limits.RequestsPerMinute = req.ApiLimits.RequestsPerMinute

		limits.MonthlyBudgetUSD = req.ApiLimits.MonthlyBudgetUSD
	}

	cl, err := c.service.Create(r.Context(), commands.CreateClaw{
		UserID:      userID,
		Name:        req.Name,
		Model:       req.Model,
		ChannelIDs:  channelIDs,
		ApiKeyLimit: limits,
	})
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})

		return
	}

	response.RespondOK(w, cl)
}
