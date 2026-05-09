package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"shared/pkg/jwt"
	"shared/pkg/response"
	"strings"

	"simpleClaw/internal/api/rest/dto"
	"simpleClaw/internal/api/rest/middleware"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/service/claw/commands"
	clawcapabilityservice "simpleClaw/internal/service/clawcapability"

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
	Restart(ctx context.Context, cm commands.RestartClaw) (entities.Claw, error)
	Delete(ctx context.Context, cm commands.DeleteClaw) (entities.Claw, error)
	ApprovePairing(ctx context.Context, cm commands.ApprovePairing) error
	Connect(ctx context.Context, cm commands.ConnectClaw) error
}

type Claw struct {
	service           clawService
	capabilityService clawCapabilityService
	j                 jwt.JWT
}

type clawCapabilityService interface {
	Attach(ctx context.Context, cmd clawcapabilityservice.AttachCommand) error
	Detach(ctx context.Context, userID, clawID uuid.UUID, capabilityID entities.CapabilityID) error
}

func NewClaw(service clawService, capabilityService clawCapabilityService, j jwt.JWT) *Claw {
	return &Claw{
		service:           service,
		capabilityService: capabilityService,
		j:                 j,
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
		r.Post("/claws/{id}/restart", c.Restart)
		r.Post("/claws/{id}/approve", c.ApprovePairing)
		r.Post("/claws/{id}/connect", c.Connect)
		r.Put("/claws/{id}/capabilities/{capability}", c.AttachCapability)
		r.Delete("/claws/{id}/capabilities/{capability}", c.DetachCapability)
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
		UserID:       userID,
		Name:         req.Name,
		Model:        req.Model,
		ChannelIDs:   channelIDs,
		ApiKeyLimit:  limits,
		Capabilities: mapCreateCapabilities(req.Capabilities),
	})
	if err != nil {
		respondServiceError(w, err)

		return
	}

	response.RespondOK(w, clawResponse(cl))
}

func mapCreateCapabilities(req *dto.CreateClawCapabilitiesRequest) commands.CreateCapabilitySet {
	if req == nil {
		return commands.CreateCapabilitySet{}
	}

	out := commands.CreateCapabilitySet{}
	if req.WebSearch != nil {
		out.WebSearch = &commands.WebSearchCapabilityInput{
			Enabled:  req.WebSearch.Enabled,
			Provider: req.WebSearch.Provider,
		}
	}

	if req.FilesImages != nil {
		out.FilesImages = &commands.ToggleCapabilityInput{Enabled: req.FilesImages.Enabled}
	}

	if req.Memory != nil {
		out.Memory = &commands.ToggleCapabilityInput{Enabled: req.Memory.Enabled}
	}

	if req.Gmail != nil {
		out.Gmail = mapIntegrationBoundCapability(req.Gmail)
	}

	if req.GoogleCalendar != nil {
		out.GoogleCalendar = mapIntegrationBoundCapability(req.GoogleCalendar)
	}

	if req.Sheets != nil {
		out.Sheets = mapIntegrationBoundCapability(req.Sheets)
	}

	return out
}

func mapIntegrationBoundCapability(req *dto.IntegrationBoundCapabilityRequest) *commands.IntegrationBoundCapabilityInput {
	if req == nil {
		return nil
	}

	var accountIntegrationID *uuid.UUID
	if raw := strings.TrimSpace(req.AccountIntegrationID); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			accountIntegrationID = &parsed
		}
	}

	return &commands.IntegrationBoundCapabilityInput{
		Enabled:              req.Enabled,
		Provider:             req.Provider,
		AccountIntegrationID: accountIntegrationID,
	}
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
		rs = append(rs, clawResponse(cl))
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

	response.RespondOK(w, clawResponse(cl))
}

func stringifyUUID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}

	return id.String()
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

	response.RespondOK(w, clawResponse(cl))
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

	cl, err := c.service.Delete(r.Context(), commands.DeleteClaw{
		UserID:       userID,
		ClawID:       id,
		DeleteConfig: true,
	})
	if err != nil {
		respondServiceError(w, err)

		return
	}

	respondAccepted(w, clawResponse(cl))
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

	cl, err := c.service.Start(r.Context(), commands.StartClaw{
		UserID: userID,
		ClawID: id,
	})
	if err != nil {
		respondServiceError(w, err)

		return
	}

	respondAccepted(w, clawResponse(cl))
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

	cl, err := c.service.Stop(r.Context(), commands.StopClaw{
		UserID: userID,
		ClawID: id,
	})
	if err != nil {
		respondServiceError(w, err)

		return
	}

	respondAccepted(w, clawResponse(cl))
}

func (c *Claw) Restart(w http.ResponseWriter, r *http.Request) {
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

	cl, err := c.service.Restart(r.Context(), commands.RestartClaw{
		UserID: userID,
		ClawID: id,
	})
	if err != nil {
		respondServiceError(w, err)

		return
	}

	respondAccepted(w, clawResponse(cl))
}

func clawResponse(cl entities.Claw) dto.ClawResponse {
	resolvedModel := clawPrimaryModel(cl.Config)

	return dto.ClawResponse{
		ID:                 cl.ID.String(),
		Name:               cl.Name,
		ModelKey:           dashboardModelKey(resolvedModel),
		ResolvedModel:      resolvedModel,
		DesiredState:       string(cl.DesiredState),
		ObservedState:      string(cl.ObservedState),
		LifecycleStatus:    string(cl.LifecycleStatus),
		CurrentOperationID: stringifyUUID(cl.CurrentOperationID),
		LastError:          cl.LastError,
	}
}

func clawPrimaryModel(cfg entities.ClawConfig) string {
	for _, agent := range cfg.Agents.List {
		if agent.Model == nil {
			continue
		}

		model := strings.TrimSpace(agent.Model.Primary)
		if model == "" {
			continue
		}

		if agent.Default {
			return model
		}
	}

	for _, agent := range cfg.Agents.List {
		if agent.Model == nil {
			continue
		}

		model := strings.TrimSpace(agent.Model.Primary)
		if model != "" {
			return model
		}
	}

	return ""
}

func dashboardModelKey(resolved string) string {
	switch {
	case strings.Contains(resolved, "claude-sonnet-4.6"):
		return "claude-sonnet-4-6"
	case strings.Contains(resolved, "gpt-5.4"):
		return "openai-gpt-5-4"
	case strings.Contains(resolved, "gemini-3-flash"):
		return "gemini-3-flash"
	case strings.Contains(resolved, "qwen3.5-flash"):
		return "qwen-3-5-flash"
	case strings.Contains(resolved, "kimi-k2.5"):
		return "kimi-k2-5"
	case strings.Contains(resolved, "glm-5-turbo"):
		return "glm-5-turbo"
	default:
		return ""
	}
}

func respondAccepted(w http.ResponseWriter, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write(raw)
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
		UserID:      userID,
		ClawID:      id,
		Code:        code,
		ChannelType: strings.TrimSpace(req.ChannelType),
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

func (c *Claw) AttachCapability(w http.ResponseWriter, r *http.Request) {
	if c.capabilityService == nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusServiceUnavailable,
			Message: "capability service is not configured",
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

	clawID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid claw id",
		})

		return
	}

	capabilityID, err := clawcapabilityservice.ParseCapabilityID(strings.TrimSpace(chi.URLParam(r, "capability")))
	if err != nil {
		respondServiceError(w, err)

		return
	}

	var req dto.AttachClawCapabilityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid request body",
		})

		return
	}

	var accountIntegrationID *uuid.UUID
	if raw := strings.TrimSpace(req.AccountIntegrationID); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.RespondError(w, response.Error{
				Code:    http.StatusBadRequest,
				Message: "invalid account integration id",
			})

			return
		}

		accountIntegrationID = &id
	}

	if err := c.capabilityService.Attach(r.Context(), clawcapabilityservice.AttachCommand{
		UserID:               userID,
		ClawID:               clawID,
		CapabilityID:         capabilityID,
		Provider:             strings.TrimSpace(req.Provider),
		AccountIntegrationID: accountIntegrationID,
		Enabled:              req.Enabled,
		Settings:             req.Settings,
	}); err != nil {
		respondServiceError(w, err)

		return
	}

	response.RespondOK(w, map[string]any{"attached": true})
}

func (c *Claw) DetachCapability(w http.ResponseWriter, r *http.Request) {
	if c.capabilityService == nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusServiceUnavailable,
			Message: "capability service is not configured",
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

	clawID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid claw id",
		})

		return
	}

	capabilityID, err := clawcapabilityservice.ParseCapabilityID(strings.TrimSpace(chi.URLParam(r, "capability")))
	if err != nil {
		respondServiceError(w, err)

		return
	}

	if err := c.capabilityService.Detach(r.Context(), userID, clawID, capabilityID); err != nil {
		respondServiceError(w, err)

		return
	}

	response.RespondOK(w, map[string]any{"detached": true})
}
