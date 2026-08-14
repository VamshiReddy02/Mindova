package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrEmptyMessages = errors.New("llm: no messages provided")

type httpChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type httpChatCompletionRequest struct {
	Messages []httpChatMessage `json:"messages"`
}

type httpChatCompletionResponse struct {
	Message httpChatMessage `json:"message"`
}

type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(baseURL string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &HTTPClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *HTTPClient) Complete(ctx context.Context, messages []Message) (string, error) {
	if len(messages) == 0 {
		return "", ErrEmptyMessages
	}

	reqBody := httpChatCompletionRequest{
		Messages: make([]httpChatMessage, len(messages)),
	}
	for i, m := range messages {
		reqBody.Messages[i] = httpChatMessage{Role: m.Role, Content: m.Content}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llm: failed to encode request: %w", err)
	}

	url := c.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: request to AI service failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm: failed to read AI service response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf(
			"llm: AI service returned status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(respBody)),
		)
	}

	var parsed httpChatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("llm: failed to parse AI service response JSON: %w", err)
	}

	if parsed.Message.Content == "" {
		return "", errors.New("llm: AI service returned an empty answer")
	}

	return parsed.Message.Content, nil
}

// Compile-time check that *HTTPClient satisfies Client.
var _ Client = (*HTTPClient)(nil)
