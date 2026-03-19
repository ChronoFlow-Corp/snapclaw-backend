package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"shared/pkg/jwt"
	"shared/pkg/response"
	"strings"

	"simpleClaw/internal/api/rest/middleware"

	"simpleClaw/internal/api/rest/dto"
	"simpleClaw/internal/entities"
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
	ApprovePairing(ctx context.Context, cm commands.ApprovePairing) error
	Connect(ctx context.Context, cm commands.ConnectClaw) error
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
		r.Post("/claws/{id}/approve", c.ApprovePairing)
		r.Post("/claws/{id}/connect", c.Connect)
		r.Delete("/claws/{id}", c.Delete)
	})
}

func (c *Claw) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateClawRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid request body",
		})

		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})

		return
	}

	channelIDs := make([]uuid.UUID, 0, len(req.ChannelIDs))
	for _, raw := range req.ChannelIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.RespondError(w, response.Error{
				Code:    http.StatusBadRequest,
				Message: "invalid channel id",
			})

			return
		}

		channelIDs = append(channelIDs, id)
	}

	var limits commands.ApiKeyLimits
	if req.ApiLimits != nil {
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
		respondServiceError(w, err)

		return
	}

	response.RespondOK(w, dto.CreateClawResponse{
		ID:     cl.ID.String(),
		Name:   cl.Name,
		Status: cl.Status,
	})
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
		respondServiceError(w, err)
		return
	}

	rs := make([]dto.CreateClawResponse, 0, len(cls))

	for _, cl := range cls {
		rs = append(rs, dto.CreateClawResponse{
			ID:     cl.ID.String(),
			Name:   cl.Name,
			Status: cl.Status,
		})
	}

	response.RespondOK(w, rs)
}

func (c *Claw) Get(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid claw id",
		})
		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})
		return
	}

	cl, err := c.service.GetByID(r.Context(), id, userID)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, dto.CreateClawResponse{
		ID:     cl.ID.String(),
		Name:   cl.Name,
		Status: cl.Status,
	})
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
			Message: "invalid request body",
		})
		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})
		return
	}

	var channelIDs []uuid.UUID
	if req.ChannelIDs != nil {
		channelIDs = make([]uuid.UUID, 0, len(req.ChannelIDs))
		for _, raw := range req.ChannelIDs {
			chID, err := uuid.Parse(raw)
			if err != nil {
				response.RespondError(w, response.Error{
					Code:    http.StatusBadRequest,
					Message: "invalid channel id",
				})
				return
			}

			channelIDs = append(channelIDs, chID)
		}
	}

	var limits commands.ApiKeyLimits
	if req.ApiLimits != nil {
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
		respondServiceError(w, err)
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
			Message: "invalid claw id",
		})
		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})
		return
	}

	if err := c.service.Delete(r.Context(), commands.DeleteClaw{
		UserID:       userID,
		ClawID:       id,
		DeleteConfig: true,
	}); err != nil {
		respondServiceError(w, err)
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
			Message: "invalid claw id",
		})

		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})

		return
	}

	_, err = c.service.Start(r.Context(), commands.StartClaw{
		UserID: userID,
		ClawID: id,
	})
	if err != nil {
		respondServiceError(w, err)
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
			Message: "invalid claw id",
		})

		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})

		return
	}

	_, err = c.service.Stop(r.Context(), commands.StopClaw{
		UserID: userID,
		ClawID: id,
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, map[string]any{"stopped": true})
}

func (c *Claw) ApprovePairing(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid claw id",
		})

		return
	}

	var req dto.ApprovePairingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid request body",
		})

		return
	}

	code := strings.TrimSpace(req.Code)
	if code == "" {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "code is required",
		})

		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})

		return
	}

	if err := c.service.ApprovePairing(r.Context(), commands.ApprovePairing{
		UserID: userID,
		ClawID: id,
		Code:   code,
	}); err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, map[string]any{"approved": true})
}

func (c *Claw) Connect(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid claw id",
		})

		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})

		return
	}

	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	if provider == "" {
		provider = "gmail"
	}

	if err := c.service.Connect(r.Context(), commands.ConnectClaw{
		UserID:   userID,
		ClawID:   id,
		Provider: provider,
	}); err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, map[string]any{"connected": true})
}
