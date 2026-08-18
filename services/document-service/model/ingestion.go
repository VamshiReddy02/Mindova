package model

import "time"

type IngestionStatus string

const (
	IngestionPending    IngestionStatus = "pending"
	IngestionProcessing IngestionStatus = "processing"
	IngestionCompleted  IngestionStatus = "completed"
	IngestionFailed     IngestionStatus = "failed"
)

type Ingestion struct {
	ID          string          `json:"id"`
	DocumentID  string          `json:"document_id"`
	Status      IngestionStatus `json:"status"`
	Error       string          `json:"error,omitempty"`
	Attempts    int             `json:"attempts"`
	LastError   string          `json:"last_error,omitempty"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	ProcessedAt *time.Time      `json:"processed_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}
