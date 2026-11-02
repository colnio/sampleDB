-- Migration 013: AFM working-hours restriction.
-- Defaults to true, so every user stays unrestricted until an admin unticks the
-- flag on their user card.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS afm_outside_hours_allowed BOOLEAN NOT NULL DEFAULT true;

-- The regular schedule: a single row. Minutes are counted from midnight;
-- weekdays use Go's numbering (0 = Sunday). Defaults to Mon-Fri 08:00-18:00.
CREATE TABLE IF NOT EXISTS working_hours (
    id           SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    start_minute INT NOT NULL DEFAULT 480,
    end_minute   INT NOT NULL DEFAULT 1080,
    weekdays     INT[] NOT NULL DEFAULT '{1,2,3,4,5}',
    CHECK (start_minute >= 0 AND end_minute < 1440 AND end_minute > start_minute)
);

INSERT INTO working_hours (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- One-day overrides such as holidays. NULL times mean no working hours that day.
CREATE TABLE IF NOT EXISTS working_hours_exceptions (
    exception_date DATE PRIMARY KEY,
    start_minute   INT,
    end_minute     INT,
    note           TEXT NOT NULL DEFAULT '',
    CHECK (
        (start_minute IS NULL AND end_minute IS NULL)
        OR (start_minute IS NOT NULL AND end_minute IS NOT NULL
            AND start_minute >= 0 AND end_minute < 1440 AND end_minute > start_minute)
    )
);
