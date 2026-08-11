package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/vamshireddy02/mindova/packages/kernel/config"
	httpkernel "github.com/vamshireddy02/mindova/packages/kernel/http"
	"github.com/vamshireddy02/mindova/packages/kernel/logger"
	"github.com/vamshireddy02/mindova/packages/kernel/middleware"
)

// HealthResponse represents the /health endpoint response
type HealthResponse struct {
	Status string `json:"status"`
}

func main() {
	// 1. Load configuration from environment
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	// 2. Create logger using config
	log := logger.New(config.AppConfig{
		Environment: cfg.App.Environment,
		LogLevel:    cfg.App.LogLevel,
	})

	// 3. Create HTTP router
	mux := http.NewServeMux()

	// 4. Add endpoints
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/panic", panicHandler)

	// 5. Build middleware chain (innermost to outermost)
	// Request flow: RequestID → RequestLogging → Recovery → Handler
	handler := middleware.RequestID(
		middleware.RequestLogging(log)(
			middleware.Recovery(log)(
				mux,
			),
		),
	)

	// 6. Create Mindova HTTP server using config
	server := httpkernel.New(cfg.App, handler, log)

	// 7. Start server in background
	go func() {
		if err := server.Start(); err != nil {
			log.Error("server failed to start", "error", err.Error())
		}
	}()

	log.Info("example service started",
		"name", cfg.App.Name,
		"address", server.Address(),
	)

	// 8. Wait for shutdown signal (SIGTERM or SIGINT)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Info("shutdown signal received")

	// 9. Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "error", err.Error())
	}

	log.Info("service stopped")
}

// healthHandler handles GET /health requests
func healthHandler(w http.ResponseWriter, r *http.Request) {
	// Only allow GET
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Demonstrate retrieving request ID from context
	// (set by middleware.RequestID middleware)
	requestID := middleware.GetRequestID(r.Context())
	_ = requestID // Used only for demonstration; in production would include in response

	response := HealthResponse{
		Status: "ok",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// panicHandler intentionally panics to test recovery middleware
func panicHandler(w http.ResponseWriter, r *http.Request) {
	panic("intentional panic in /panic endpoint")
}
