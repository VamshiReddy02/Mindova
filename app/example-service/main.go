package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/vamshireddy02/mindova/packages/kernel/config"
	ServerHttp "github.com/vamshireddy02/mindova/packages/kernel/http"
	"github.com/vamshireddy02/mindova/packages/kernel/logger"
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

	// 4. Add /health endpoint
	mux.HandleFunc("/health", healthHandler)

	// 5. Create Mindova HTTP server using config
	server := ServerHttp.New(cfg.App, mux, log)

	// 6. Start server in background
	go func() {
		if err := server.Start(); err != nil {
			log.Error("server failed to start", "error", err.Error())
		}
	}()

	log.Info("example service started",
		"name", cfg.App.Name,
		"address", server.Address(),
	)

	// 7. Wait for shutdown signal (SIGTERM or SIGINT)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Info("shutdown signal received")

	// 8. Graceful shutdown with timeout
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

	response := HealthResponse{
		Status: "ok",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
