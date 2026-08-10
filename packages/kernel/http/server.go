package ServerHttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/vamshireddy02/mindova/packages/kernel/config"
	"github.com/vamshireddy02/mindova/packages/kernel/logger"
)

type Server struct {
	httpServer *http.Server
	logger     *logger.Logger
}

func New(cfg config.AppConfig, handler http.Handler, log *logger.Logger) *Server {

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	httpSrv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	return &Server{
		httpServer: httpSrv,
		logger:     log,
	}
}

func (s *Server) Start() error {
	s.logger.Info("HTTP server starting",
		"address", s.httpServer.Addr,
	)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		s.logger.Error("HTTP server failed to start", "error", err)
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("HTTP server shutting down")

	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("HTTP server shutdown error", "error", err)
		return fmt.Errorf("shutdown error: %w", err)
	}

	s.logger.Info("HTTP server stopped")
	return nil
}

func (s *Server) Address() string {
	return s.httpServer.Addr
}
