package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

// IngestionRepository defines the persistence operations for ingestion jobs.
type IngestionRepository interface {
	Create(ctx context.Context, ing *model.Ingestion) error
	GetByID(ctx context.Context, id string) (*model.Ingestion, error)
	GetByDocumentID(ctx context.Context, documentID string) ([]*model.Ingestion, error)
	ListPending(ctx context.Context, limit int) ([]*model.Ingestion, error)
	UpdateStatus(ctx context.Context, id string, status model.IngestionStatus, errMsg string) error
}

// IngestionRepo is the PostgreSQL-backed implementation of IngestionRepository.
// Named distinctly from Repository (which handles documents) so a service
// can depend on both side by side without a naming collision.
type IngestionRepo struct {
	db *pgxpool.Pool
}

// NewIngestionRepo creates an IngestionRepo backed by the given connection pool.
func NewIngestionRepo(db *pgxpool.Pool) *IngestionRepo {
	return &IngestionRepo{db: db}
}

// Create inserts an ingestion job into PostgreSQL. PostgreSQL generates the
// ID and created_at values via gen_random_uuid()/now(); Create populates
// them back onto ing after the insert.
func (r *IngestionRepo) Create(ctx context.Context, ing *model.Ingestion) error {
	const query = `
		INSERT INTO ingestions (id, document_id, status, error, started_at, completed_at)
		VALUES (gen_random_uuid(), $1, $2, NULLIF($3, ''), $4, $5)
		RETURNING id, created_at
	`

	return r.db.QueryRow(ctx, query,
		ing.DocumentID,
		ing.Status,
		ing.Error,
		ing.StartedAt,
		ing.CompletedAt,
	).Scan(&ing.ID, &ing.CreatedAt)
}

// GetByID fetches a single ingestion job by its ID.
// Returns ErrNotFound if no ingestion exists with that ID.
func (r *IngestionRepo) GetByID(ctx context.Context, id string) (*model.Ingestion, error) {
	const query = `
		SELECT id, document_id, status, COALESCE(error, ''), started_at, completed_at, created_at
		FROM ingestions
		WHERE id = $1
	`

	ing := &model.Ingestion{}

	err := r.db.QueryRow(ctx, query, id).Scan(
		&ing.ID,
		&ing.DocumentID,
		&ing.Status,
		&ing.Error,
		&ing.StartedAt,
		&ing.CompletedAt,
		&ing.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return ing, nil
}

// GetByDocumentID fetches every ingestion attempt for a document, most
// recent first — preserving the full retry history rather than only the
// latest attempt.
func (r *IngestionRepo) GetByDocumentID(ctx context.Context, documentID string) ([]*model.Ingestion, error) {
	const query = `
		SELECT id, document_id, status, COALESCE(error, ''), started_at, completed_at, created_at
		FROM ingestions
		WHERE document_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ingestions := make([]*model.Ingestion, 0)

	for rows.Next() {
		ing := &model.Ingestion{}
		if err := rows.Scan(
			&ing.ID,
			&ing.DocumentID,
			&ing.Status,
			&ing.Error,
			&ing.StartedAt,
			&ing.CompletedAt,
			&ing.CreatedAt,
		); err != nil {
			return nil, err
		}
		ingestions = append(ingestions, ing)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ingestions, nil
}

// ListPending fetches up to limit ingestions in the "pending" state,
// oldest first — so the worker processes jobs in the order they were
// requested (FIFO) rather than newest-first.
func (r *IngestionRepo) ListPending(ctx context.Context, limit int) ([]*model.Ingestion, error) {
	const query = `
		SELECT id, document_id, status, COALESCE(error, ''), started_at, completed_at, created_at
		FROM ingestions
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ingestions := make([]*model.Ingestion, 0)

	for rows.Next() {
		ing := &model.Ingestion{}
		if err := rows.Scan(
			&ing.ID,
			&ing.DocumentID,
			&ing.Status,
			&ing.Error,
			&ing.StartedAt,
			&ing.CompletedAt,
			&ing.CreatedAt,
		); err != nil {
			return nil, err
		}
		ingestions = append(ingestions, ing)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ingestions, nil
}

// UpdateStatus transitions an ingestion job to a new status.
//
// It also manages the lifecycle timestamps automatically:
//   - moving into "processing" sets started_at (only the first time)
//   - moving into "completed" or "failed" sets completed_at
//
// errMsg is stored as the failure reason; pass "" when there is none
// (e.g. transitioning to processing or completed).
// Returns ErrNotFound if no ingestion exists with that ID.
func (r *IngestionRepo) UpdateStatus(ctx context.Context, id string, status model.IngestionStatus, errMsg string) error {
	const query = `
		UPDATE ingestions
		SET
			status = $1::text,
			error = NULLIF($2, ''),
			started_at = CASE
				WHEN $1::text = 'processing' AND started_at IS NULL THEN now()
				ELSE started_at
			END,
			completed_at = CASE
				WHEN $1::text IN ('completed', 'failed') THEN now()
				ELSE completed_at
			END
		WHERE id = $3
	`

	tag, err := r.db.Exec(ctx, query, status, errMsg, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Compile-time check that *IngestionRepo satisfies IngestionRepository.
var _ IngestionRepository = (*IngestionRepo)(nil)
