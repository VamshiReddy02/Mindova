package handler

import (
	"errors"
	"net/http"

	"github.com/vamshireddy02/mindova/services/document-service/repository"
)

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	id := extractID(r, "/documents/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing document is")
		return
	}

	err := h.svc.Delete(r.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete document")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
