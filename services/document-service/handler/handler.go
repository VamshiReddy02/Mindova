package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/vamshireddy02/mindova/services/document-service/service"
)

type Handler struct {
	svc service.DocumentService
}

func New(svc service.DocumentService) *Handler {
	return &Handler{svc: svc}
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func extractID(r *http.Request, prefix string) string {
	id := strings.TrimPrefix(r.URL.Path, prefix)
	id = strings.Trim(id, "/")
	return id
}
