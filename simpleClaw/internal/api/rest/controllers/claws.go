package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"shared/pkg/jwt"
	"shared/pkg/response"

	"simpleClaw/internal/api/rest/middleware"

	"simpleClaw/internal/api/rest/dto"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/claw/commands"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type clawService interface {
	Create(ctx context.Context, cm commands.CreateClaw) (entities.Claw, error)
	GetByID(ctx context.Context, clawID uuid.UUID, userID uuid.UUID) (entities.Claw, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.Claw, error)
	Update(ctx context.Context, cm commands.UpdateClaw) (entities.Claw, error)
	Start(ctx context.Context, cm commands.StartClaw) (entities.Claw, error)
	Stop(ctx context.Context, cm commands.StopClaw) (entities.Claw, error)
	Delete(ctx context.Context, cm commands.DeleteClaw) error
}

type Claw struct {
	service clawService
	j       jwt.JWT
}

func NewClaw(service clawService, j jwt.JWT) *Claw {
	return &Claw{
		service: service,
		j:       j,
	}
}

func (c *Claw) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthJwt(c.j))
		r.Get("/claws", c.List)
		r.Get("/claws/{id}", c.Get)
		r.Post("/claws", c.Create)
		r.Put("/claws/{id}", c.Update)
		r.Post("/claws/{id}/start", c.Start)
		r.Post("/claws/{id}/stop", c.Stop)
		r.Delete("/claws/{id}", c.Delete)
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

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "User ID should be a UUID",
		})

		return
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

func (c *Claw) List(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "User ID should be a UUID",
		})
		return
	}

	cls, err := c.service.GetByUserID(r.Context(), userID)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNotFound) {
			code = http.StatusNotFound
		}

		response.RespondError(w, response.Error{
			Code:    code,
			Message: err.Error(),
		})
		return
	}

	response.RespondOK(w, cls)
}

func (c *Claw) Get(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid claw id: " + rawID,
		})
		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "User ID should be a UUID",
		})
		return
	}

	cl, err := c.service.GetByID(r.Context(), id, userID)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNotFound) {
			code = http.StatusNotFound
		}

		response.RespondError(w, response.Error{
			Code:    code,
			Message: err.Error(),
		})
		return
	}

	response.RespondOK(w, cl)
}

func (c *Claw) Update(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid claw id: " + rawID,
		})
		return
	}

	var req dto.UpdateClawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "User ID should be a UUID",
		})
		return
	}

	channelIDs := make([]uuid.UUID, 0, len(req.ChannelIDs))
	for _, raw := range req.ChannelIDs {
		chID, err := uuid.Parse(raw)
		if err != nil {
			response.RespondError(w, response.Error{
				Code:    http.StatusBadRequest,
				Message: "invalid channel id: " + raw,
			})
			return
		}

		channelIDs = append(channelIDs, chID)
	}

	var limits commands.ApiKeyLimits
	if req.ApiLimits != nil {
		limits.RequestsPerMinute = req.ApiLimits.RequestsPerMinute
		limits.MonthlyBudgetUSD = req.ApiLimits.MonthlyBudgetUSD
	}

	cl, err := c.service.Update(r.Context(), commands.UpdateClaw{
		UserID:      userID,
		ClawID:      id,
		Name:        req.Name,
		Model:       req.Model,
		ChannelIDs:  channelIDs,
		ApiKeyLimit: limits,
	})
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNotFound) {
			code = http.StatusNotFound
		}

		response.RespondError(w, response.Error{
			Code:    code,
			Message: err.Error(),
		})
		return
	}

	response.RespondOK(w, cl)
}

func (c *Claw) Delete(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid claw id: " + rawID,
		})
		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "User ID should be a UUID",
		})
		return
	}

	if err := c.service.Delete(r.Context(), commands.DeleteClaw{
		UserID: userID,
		ClawID: id,
	}); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNotFound) {
			code = http.StatusNotFound
		}

		response.RespondError(w, response.Error{
			Code:    code,
			Message: err.Error(),
		})
		return
	}

	response.RespondOK(w, map[string]any{"deleted": true})
}

func (c *Claw) Start(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid claw id: " + rawID,
		})

		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "User ID should be a UUID",
		})

		return
	}

	_, err = c.service.Start(r.Context(), commands.StartClaw{
		UserID: userID,
		ClawID: id,
	})
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNotFound) {
			code = http.StatusNotFound
		}

		response.RespondError(w, response.Error{
			Code:    code,
			Message: err.Error(),
		})

		return
	}

	response.RespondOK(w, map[string]any{"started": true})
}

func (c *Claw) Stop(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid claw id: " + rawID,
		})

		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "User ID should be a UUID",
		})

		return
	}

	_, err = c.service.Stop(r.Context(), commands.StopClaw{
		UserID: userID,
		ClawID: id,
	})
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNotFound) {
			code = http.StatusNotFound
		}

		response.RespondError(w, response.Error{
			Code:    code,
			Message: err.Error(),
		})

		return
	}

	response.RespondOK(w, map[string]any{"stopped": true})
}
