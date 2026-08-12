package handler

import (
	"errors"
	"net/http"

	"github.com/vamshireddy02/mindova/services/document-service/repository"
)

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	id := extractID(r, "/documents/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing document id")
		return
	}

	doc, err := h.svc.GetByID(r.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch document")
		return
	}

	writeJSON(w, http.StatusOK, doc)

}
