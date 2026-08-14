package handler

import (
	"errors"
	"net/http"

	"github.com/vamshireddy02/mindova/services/document-service/service"
)

// searchRequest is the expected JSON body for POST /documents/search.
type searchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// Search handles POST /documents/search.
//
// Flow:
//
//	HTTP request -> decode -> RetrievalService.Search() ->
//	embed query -> pgvector SearchSimilar() -> top-K chunks -> JSON response
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req searchRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	chunks, err := h.retrieval.Search(r.Context(), req.Query, req.Limit)
	switch {
	case errors.Is(err, service.ErrEmptyQuery):
		writeError(w, http.StatusBadRequest, "query must not be empty")
		return
	case errors.Is(err, service.ErrInvalidLimit):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	writeJSON(w, http.StatusOK, chunks)
}
