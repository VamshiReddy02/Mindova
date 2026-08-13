CREATE TABLE IF NOT EXISTS ingestions (
    id UUID PRIMARY KEY,
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL,
    error TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ingestions_status_check
        CHECK (status IN (
            'pending',
            'processing',
            'completed',
            'failed'
        ))
);

CREATE INDEX IF NOT EXISTS idx_ingestions_document_id
    ON ingestions(document_id);

CREATE INDEX IF NOT EXISTS idx_ingestions_status
    ON ingestions(status);