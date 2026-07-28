package rest

import (
	"context"
	"net/http"
	"time"
)

type Server struct {
	s *http.Server
}

func NewServer(addr string, handlers http.Handler) *Server {
	s := &http.Server{}

	s.Addr = addr
	s.Handler = handlers

	s.ReadHeaderTimeout = 5 * time.Second
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
