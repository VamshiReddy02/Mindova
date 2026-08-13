package embedding

import (
	"context"
	"errors"
	"hash/fnv"
)

const defaultDimensions = 8

var ErrEmptyTexts = errors.New("embedding: no texts provided")

type MockEmbedder struct {
	Dimensions int
	Err        error
}

// Embed returns one deterministic fake vector per input text.
func (m *MockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if len(texts) == 0 {
		return nil, ErrEmptyTexts
	}

	dims := m.Dimensions
	if dims <= 0 {
		dims = defaultDimensions
	}

	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vectors[i] = fakeVector(text, dims)
	}
	return vectors, nil
}

func fakeVector(text string, dims int) []float32 {
	vec := make([]float32, dims)

	for d := 0; d < dims; d++ {
		h := fnv.New32a()
		h.Write([]byte(text))
		h.Write([]byte{byte(d)})
		sum := h.Sum32()

		vec[d] = float32(sum%2000)/1000.0 - 1.0
	}

	return vec
}

var _ Embedder = (*MockEmbedder)(nil)
