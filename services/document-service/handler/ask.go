package handler

import (
	"errors"
	"net/http"

	"github.com/vamshireddy02/mindova/services/document-service/model"
	"github.com/vamshireddy02/mindova/services/document-service/service"
)

type askRequest struct {
	Question string `json:"question"`
	Limit    int    `json:"limit"`
}

type askSource struct {
	DocumentID string `json:"document_id"`
	ChunkIndex int    `json:"chunk_index"`
	Content    string `json:"content"`
}

type askResponse struct {
	Answer  string      `json:"answer"`
	Sources []askSource `json:"sources"`
}

func (h *Handler) Ask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req askRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	result, err := h.rag.Ask(r.Context(), req.Question, req.Limit)
	switch {
	case errors.Is(err, service.ErrEmptyQuery):
		writeError(w, http.StatusBadRequest, "question must not be empty")
		return
	case errors.Is(err, service.ErrInvalidLimit):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to generate answer")
		return
	}

	writeJSON(w, http.StatusOK, askResponse{
		Answer:  result.Answer,
		Sources: toAskSources(result.Chunks),
	})
}

func toAskSources(chunks []*model.DocumentChunk) []askSource {
	sources := make([]askSource, len(chunks))
	for i, c := range chunks {
		sources[i] = askSource{
			DocumentID: c.DocumentID,
			ChunkIndex: c.ChunkIndex,
			Content:    c.Content,
		}
	}
	return sources
}
