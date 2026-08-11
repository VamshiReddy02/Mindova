package model

import (
	"testing"
	"time"
)

func TestDocument(t *testing.T) {
	now := time.Now()
	doc := Document{
		ID:          "doc-123",
		Name:        "architecture.md",
		Content:     "Mindova is an AI knowledge platform.",
		ContentType: "text/markdown",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if doc.ID != "doc-123" {
		t.Errorf("expected ID doc-123, got %s", doc.ID)
	}
	if doc.Name != "architecture.md" {
		t.Errorf("expected name architecture.md, got %s", doc.Name)
	}
	if doc.ContentType != "text/markdown" {
		t.Errorf("expected content type text/markdown, got %s", doc.ContentType)
	}
}
