package handler

import (
	"fmt"
	"net/http"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

type createDocumentRequest struct {
	Name        string `json:"name"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req createDocumentRequest

	if !decodeJSON(w, r, &req) {
		return
	}

	if err := validateCreateDocumentRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	doc := &model.Document{
		Name:        req.Name,
		Content:     req.Content,
		ContentType: req.ContentType,
	}

	if err := h.svc.Create(r.Context(), doc); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create document")
		return
	}

	ingestion := &model.Ingestion{DocumentID: doc.ID}
	if err := h.ingestion.Create(r.Context(), ingestion); err != nil {
		h.log.Error("failed to enqueue ingestion",
			"document_id", doc.ID,
			"error", err.Error(),
		)
	}

	writeJSON(w, http.StatusCreated, doc)
}

// validateCreateDocumentRequest checks required fields are present.
func validateCreateDocumentRequest(req createDocumentRequest) error {
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
