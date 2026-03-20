package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"shared/pkg/jwt"
	"shared/pkg/response"

	"simpleClaw/internal/api/rest/dto"
	"simpleClaw/internal/api/rest/middleware"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/service/server/commands"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type serverService interface {
	Create(ctx context.Context, cm commands.CreateServer) (entities.Server, error)
	GetAll(ctx context.Context) ([]entities.Server, error)
	GetByID(ctx context.Context, id uuid.UUID) (entities.Server, error)
	Update(ctx context.Context, cm commands.UpdateServer) (entities.Server, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type adminService interface {
	UserInfo(ctx context.Context, userID uuid.UUID) (entities.User, error)
}

type Server struct {
	service serverService
	users   adminService
	j       jwt.JWT
}

func NewServer(service serverService, users adminService, j jwt.JWT) *Server {
	return &Server{
		service: service,
		users:   users,
		j:       j,
	}
}

func (s *Server) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthJwt(s.j))
		r.Use(middleware.AdminOnly(s.users))
		r.Get("/servers", s.List)
		r.Get("/servers/{id}", s.Get)
		r.Post("/servers", s.Create)
		r.Put("/servers/{id}", s.Update)
		r.Delete("/servers/{id}", s.Delete)
	})
}

func (s *Server) List(w http.ResponseWriter, r *http.Request) {
	servers, err := s.service.GetAll(r.Context())
	if err != nil {
		respondServiceError(w, err)
		return
	}

	resp := make([]dto.ServerResponse, 0, len(servers))
	for _, srv := range servers {
		resp = append(resp, serverToResponse(srv))
	}

	response.RespondOK(w, resp)
}

func (s *Server) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid server id",
		})

		return
	}

	srv, err := s.service.GetByID(r.Context(), id)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, serverToResponse(srv))
}

func (s *Server) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateServerRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid request body",
		})

		return
	}

	srv, err := s.service.Create(r.Context(), commands.CreateServer{
		Name:      req.Name,
		IP:        req.IP,
		URL:       req.URL,
		ProxyURL:  req.ProxyURL,
		Status:    req.Status,
		SecretKey: req.SecretKey,
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, serverToResponse(srv))
}

func (s *Server) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid server id",
		})

		return
	}

	var req dto.UpdateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid request body",
		})

		return
	}

	srv, err := s.service.Update(r.Context(), commands.UpdateServer{
		ID:        id,
		Name:      req.Name,
		IP:        req.IP,
		URL:       req.URL,
		ProxyURL:  req.ProxyURL,
		Status:    req.Status,
		SecretKey: req.SecretKey,
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, serverToResponse(srv))
}

func (s *Server) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid server id",
		})

		return
	}

	if err := s.service.Delete(r.Context(), id); err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, map[string]bool{"deleted": true})
}

func serverToResponse(srv entities.Server) dto.ServerResponse {
	return dto.ServerResponse{
		ID:        srv.ID.String(),
		Name:      srv.Name,
		IP:        srv.IP,
		URL:       srv.URL,
		ProxyURL:  srv.ProxyURL,
		Status:    srv.Status,
		SecretKey: srv.SecretKey,
		MaxClaws:  srv.MaxClaws,
		CreatedAt: srv.CreatedAt,
	}
}
