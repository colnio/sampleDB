-- Migration 001: Add fabrication_steps table
CREATE TABLE IF NOT EXISTS fabrication_steps (
    step_id        SERIAL PRIMARY KEY,
    sample_id      INT NOT NULL REFERENCES samples(sample_id) ON DELETE CASCADE,
    step_order     INT NOT NULL DEFAULT 0,
    step_date      DATE,
    person_user_id INT REFERENCES users(user_id) ON DELETE SET NULL,
    description    TEXT,
    equipment_id   INT REFERENCES equipment(equipment_id) ON DELETE SET NULL,
    created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
