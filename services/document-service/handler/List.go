package handler

import (
	"net/http"
	"strconv"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	limit := defaultLimit

	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}

		if parsed <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be greater than 0")
			return
		}

		if parsed > maxLimit {
			writeError(w, http.StatusBadRequest, "limit must not exceed 100")
			return
		}

		limit = parsed
	}

	docs, err := h.svc.List(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list documents")
		return
	}

	writeJSON(w, http.StatusOK, docs)
}
