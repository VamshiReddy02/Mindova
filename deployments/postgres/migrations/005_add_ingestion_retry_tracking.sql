ALTER TABLE ingestions
    ADD COLUMN attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN last_error TEXT,
    ADD COLUMN processed_at TIMESTAMPTZ;
