package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/vamshireddy02/mindova/services/document-service/service"
)

const maxJSONBodySize = 1 << 20 // 1 MB

type Handler struct {
	svc       service.DocumentService
	retrieval service.RetrievalService
}

func New(svc service.DocumentService, retrieval service.RetrievalService) *Handler {
	return &Handler{svc: svc, retrieval: retrieval}
}

// errorResponse is the JSON body returned for any error response.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON writes v as a JSON response body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response with the given status code.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

// extractID pulls the {id} path segment out of a "/documents/{id}" URL.
// Returns an empty string if no id segment is present.
func extractID(r *http.Request, prefix string) string {
	id := strings.TrimPrefix(r.URL.Path, prefix)
	id = strings.Trim(id, "/")
	return id
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodySize)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return false
		}

		writeError(w, http.StatusBadRequest, "malformed JSON body")
		return false
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "request body must contain a single JSON object")
		return false
	}

	return true
}
