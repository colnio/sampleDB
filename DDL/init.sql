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
