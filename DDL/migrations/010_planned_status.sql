-- Migration 010: Add 'planned' to sample status values
ALTER TABLE samples DROP CONSTRAINT IF EXISTS chk_sample_status;

ALTER TABLE samples
    ADD CONSTRAINT chk_sample_status
    CHECK (status IN ('active', 'in_progress', 'archived', 'planned'));
