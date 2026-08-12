package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/repository"
)

type updateDocumentRequest struct {
	Name        string `json:"name"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	id := extractID(r, "/documents/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing document id")
		return
	}

	var req updateDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON body")
		return
	}

	if err := validateUpdateDocumentRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	doc := &model.Document{
		ID:          id,
		Name:        req.Name,
		Content:     req.Content,
		ContentType: req.ContentType,
	}

	err := h.svc.Update(r.Context(), doc)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update document")
		return
	}

	writeJSON(w, http.StatusOK, doc)
}

func validateUpdateDocumentRequest(req updateDocumentRequest) error {
	if req.Name == "" {
		return fmt.Errorf("missing required field: name")
	}
	if req.Content == "" {
		return fmt.Errorf("missing required field: content")
	}
	if req.ContentType == "" {
		return fmt.Errorf("missing required field: content_type")
	}
	return nil
}
