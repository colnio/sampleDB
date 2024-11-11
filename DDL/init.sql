CREATE TABLE if not exists samples (
    sample_id SERIAL PRIMARY KEY,
    sample_name VARCHAR(100) NOT NULL,
    sample_description TEXT,
    sample_prep TEXT,
    sample_keywords VARCHAR(255),
    sample_owner VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
grant all on schema public to app;
grant all on samples to app;
grant all on sequence samples_sample_id_seq to app;

insert into samples (sample_name, sample_description, sample_keywords, sample_owner) values ('T1', 'desc1', 'CNT', 'colnio');
insert into samples (sample_name, sample_description, sample_keywords, sample_owner) values ('T2', 'desc2', 'Gr', 'colnio');
insert into samples (sample_name, sample_description, sample_keywords, sample_owner) values ('T3', 'desc3', 'MAC', 'colnio');
insert into samples (sample_name, sample_description, sample_keywords, sample_owner) values ('T4', 'desc4', 'MoS2', 'colnio');

CREATE TABLE if not exists users (
    user_id SERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    is_approved BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

grant all on users to app;
grant all on sequence users_user_id_seq to app;

CREATE TABLE if not exists attachments (
    attachment_id SERIAL PRIMARY KEY,
    sample_id INT REFERENCES samples(sample_id) ON DELETE CASCADE,
    attachment_address VARCHAR(255) NOT NULL,
    uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
grant all on schema public to app;
grant all on attachments to app;
grant all on attachments_attachment_id_seq to app;

-- Wiki related tables
CREATE TABLE articles (
    article_id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL UNIQUE,
    content TEXT NOT NULL,
    created_by INT REFERENCES users(user_id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_modified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_modified_by INT REFERENCES users(user_id)
);

CREATE TABLE article_attachments (
    attachment_id SERIAL PRIMARY KEY,
    article_id INT REFERENCES articles(article_id) ON DELETE CASCADE,
    attachment_address VARCHAR(255) NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    uploaded_by INT REFERENCES users(user_id)
);

-- Booking system related tables
CREATE TABLE equipment (
    equipment_id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    location VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_equipment_permissions (
    user_id INT REFERENCES users(user_id),
    equipment_id INT REFERENCES equipment(equipment_id),
    granted_by INT REFERENCES users(user_id),
    granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, equipment_id)
);

CREATE TABLE IF NOT EXISTS bookings (
    booking_id SERIAL PRIMARY KEY,
    equipment_id INTEGER REFERENCES equipment(equipment_id),
    user_id INTEGER REFERENCES users(user_id),
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    purpose TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT valid_time_range CHECK (end_time > start_time)
);

CREATE INDEX IF NOT EXISTS idx_bookings_time_range 
ON bookings (equipment_id, start_time, end_time);

grant all on bookings to app;

-- Grant necessary permissions
GRANT ALL ON articles TO app;
GRANT ALL ON article_attachments TO app;
GRANT ALL ON equipment TO app;
GRANT ALL ON user_equipment_permissions TO app;
GRANT ALL ON bookings TO app;

GRANT ALL ON SEQUENCE articles_article_id_seq TO app;
GRANT ALL ON SEQUENCE article_attachments_attachment_id_seq TO app;
GRANT ALL ON SEQUENCE equipment_equipment_id_seq TO app;

-- Create extension for time range exclusion
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Insert some sample equipment
INSERT INTO equipment (name, description, location) VALUES
    ('SEM', 'Scanning Electron Microscope', 'Room 101'),
    ('TEM', 'Transmission Electron Microscope', 'Room 102'),
    ('XRD', 'X-Ray Diffractometer', 'Room 103'),
    ('AFM', 'Atomic Force Microscope', 'Room 104');