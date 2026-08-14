package llm

import "context"

type Message struct {
	Role    string
	Content string
}

type Client interface {
	Complete(ctx context.Context, messages []Message) (string, error)
}
