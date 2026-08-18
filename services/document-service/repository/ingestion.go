package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vamshireddy02/mindova/services/document-service/model"
)

const ingestionColumns = `
	id, document_id, status, COALESCE(error, ''), attempts,
	COALESCE(last_error, ''), started_at, completed_at, processed_at, created_at
`

// IngestionRepository defines the persistence operations for ingestion jobs.
type IngestionRepository interface {
	Create(ctx context.Context, ing *model.Ingestion) error
	GetByID(ctx context.Context, id string) (*model.Ingestion, error)
	GetByDocumentID(ctx context.Context, documentID string) ([]*model.Ingestion, error)
	ListPending(ctx context.Context, limit int) ([]*model.Ingestion, error)
	ClaimNextPending(ctx context.Context) (*model.Ingestion, error)
	UpdateStatus(ctx context.Context, id string, status model.IngestionStatus, errMsg string) error
}

type IngestionRepo struct {
	db *pgxpool.Pool
}

func NewIngestionRepo(db *pgxpool.Pool) *IngestionRepo {
	return &IngestionRepo{db: db}
}

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

// scanIngestion scans a row matching ingestionColumns' order into ing.
func scanIngestion(row pgx.Row, ing *model.Ingestion) error {
	return row.Scan(
		&ing.ID,
		&ing.DocumentID,
		&ing.Status,
		&ing.Error,
		&ing.Attempts,
		&ing.LastError,
		&ing.StartedAt,
		&ing.CompletedAt,
		&ing.ProcessedAt,
		&ing.CreatedAt,
	)
}

func (r *IngestionRepo) GetByID(ctx context.Context, id string) (*model.Ingestion, error) {
	query := `SELECT ` + ingestionColumns + ` FROM ingestions WHERE id = $1`

	ing := &model.Ingestion{}
	err := scanIngestion(r.db.QueryRow(ctx, query, id), ing)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return ing, nil
}

func (r *IngestionRepo) GetByDocumentID(ctx context.Context, documentID string) ([]*model.Ingestion, error) {
	query := `SELECT ` + ingestionColumns + ` FROM ingestions WHERE document_id = $1 ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ingestions := make([]*model.Ingestion, 0)

	for rows.Next() {
		ing := &model.Ingestion{}
		if err := scanIngestion(rows, ing); err != nil {
			return nil, err
		}
		ingestions = append(ingestions, ing)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ingestions, nil
}

func (r *IngestionRepo) ListPending(ctx context.Context, limit int) ([]*model.Ingestion, error) {
	query := `SELECT ` + ingestionColumns + ` FROM ingestions WHERE status = 'pending' ORDER BY created_at ASC LIMIT $1`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ingestions := make([]*model.Ingestion, 0)

	for rows.Next() {
		ing := &model.Ingestion{}
		if err := scanIngestion(rows, ing); err != nil {
			return nil, err
		}
		ingestions = append(ingestions, ing)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ingestions, nil
}

func (r *IngestionRepo) ClaimNextPending(ctx context.Context) (*model.Ingestion, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) // no-op once Commit succeeds

	const selectQuery = `
		SELECT id
		FROM ingestions
		WHERE status = 'pending'
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`

	var id string
	err = tx.QueryRow(ctx, selectQuery).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // nothing pending right now — not an error
	}
	if err != nil {
		return nil, err
	}

	updateQuery := `
		UPDATE ingestions
		SET
			status = 'processing',
			attempts = attempts + 1,
			started_at = COALESCE(started_at, now()),
			processed_at = now()
		WHERE id = $1
		RETURNING ` + ingestionColumns

	ing := &model.Ingestion{}
	if err := scanIngestion(tx.QueryRow(ctx, updateQuery, id), ing); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return ing, nil
}

func (r *IngestionRepo) UpdateStatus(ctx context.Context, id string, status model.IngestionStatus, errMsg string) error {
	const query = `
		UPDATE ingestions
		SET
			status = $1::text,
			error = NULLIF($2, ''),
			last_error = CASE WHEN $2 <> '' THEN $2 ELSE last_error END,
			processed_at = now(),
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
