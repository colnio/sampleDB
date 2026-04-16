-- Migration 007: Prevent overlapping bookings via exclusion constraint
-- Requires the btree_gist extension for GiST-indexable equality on integer columns.
CREATE EXTENSION IF NOT EXISTS btree_gist;

DO $$ BEGIN
    ALTER TABLE bookings
        ADD CONSTRAINT bookings_no_overlap
        EXCLUDE USING gist (
            equipment_id WITH =,
            tstzrange(start_time, end_time, '[)') WITH &&
        );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
