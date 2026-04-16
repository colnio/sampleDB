-- Migration 006: Add equipment_text column for free-text equipment entry
ALTER TABLE fabrication_steps
    ADD COLUMN IF NOT EXISTS equipment_text TEXT;
