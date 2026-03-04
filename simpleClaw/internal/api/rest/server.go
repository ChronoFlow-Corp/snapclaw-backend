package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type Server struct {
	s *http.Server
}

func NewServer(addr string, handlers chi.Router) *Server {
	s := &http.Server{}

	s.Addr = addr
	s.Handler = handlers

	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.IdleTimeout = 10 * time.Second

	return &Server{
		s: s,
	}
}

func (s *Server) ListenAndServe() error {
	return s.s.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.s.Shutdown(ctx)
}
