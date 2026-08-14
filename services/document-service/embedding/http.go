package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// httpEmbeddingRequest is the JSON body sent to POST /v1/embeddings.
type httpEmbeddingRequest struct {
	Texts []string `json:"texts"`
}

// httpEmbeddingResponse is the expected JSON body from the AI service.
type httpEmbeddingResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// HTTPEmbedder is an Embedder backed by the Python AI service, calling
// POST /v1/embeddings over HTTP. It implements the same Embedder
// interface as MockEmbedder, so callers (the worker, in particular) never
// need to know or care which one they're using.
type HTTPEmbedder struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPEmbedder creates an HTTPEmbedder targeting baseURL (e.g.
// "http://localhost:8001"). If httpClient is nil, a client with a 30s
// timeout is used.
func NewHTTPEmbedder(baseURL string, httpClient *http.Client) *HTTPEmbedder {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &HTTPEmbedder{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: httpClient,
	}
}

// Embed sends texts to the AI service's POST /v1/embeddings endpoint and
// returns the resulting vectors, in the same order as texts.
//
// Returns ErrEmptyTexts for an empty input (matching MockEmbedder's
// contract, without ever making a request), and a descriptive error for
// each of: connection failure, a non-2xx response, invalid JSON in the
// response, or an embedding count that doesn't match the input count.
func (e *HTTPEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, ErrEmptyTexts
	}

	body, err := json.Marshal(httpEmbeddingRequest{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("embedding: failed to encode request: %w", err)
	}

	url := e.baseURL + "/v1/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding: failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding: request to AI service failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embedding: failed to read AI service response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf(
			"embedding: AI service returned status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(respBody)),
		)
	}

	var parsed httpEmbeddingResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("embedding: failed to parse AI service response JSON: %w", err)
	}

	if len(parsed.Embeddings) != len(texts) {
		return nil, fmt.Errorf(
			"embedding: expected %d embeddings, got %d",
			len(texts), len(parsed.Embeddings),
		)
	}

	return parsed.Embeddings, nil
}

// Compile-time check that *HTTPEmbedder satisfies Embedder.
var _ Embedder = (*HTTPEmbedder)(nil)
