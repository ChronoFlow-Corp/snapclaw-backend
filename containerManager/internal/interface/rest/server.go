package rest

import (
	"context"
	"net/http"
)

type Server struct {
	s *http.Server
}

func NewServer(addr string, handler http.Handler) *Server {
	return &Server{
		s: &http.Server{
			Addr:    addr,
			Handler: handler,
		},
	}
}

func (s *Server) Start() error {
	return s.s.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	return s.s.Shutdown(ctx)
}
