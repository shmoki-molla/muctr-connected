-- Auth schema. This file can also be run against an existing local database.
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS auth_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    password_hash TEXT NOT NULL CHECK (password_hash <> ''),
    role VARCHAR(20) NOT NULL CHECK (role IN ('student', 'admin')),
    student_id UUID UNIQUE REFERENCES students(id) ON DELETE RESTRICT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT auth_users_role_student_link CHECK (
        (role = 'student' AND student_id IS NOT NULL)
        OR (role = 'admin' AND student_id IS NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS auth_users_email_lower_uq
    ON auth_users (lower(email));

-- The browser receives the raw random token in an HttpOnly cookie.
-- Only its SHA-256 hash is stored in PostgreSQL.
CREATE TABLE IF NOT EXISTS auth_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT auth_sessions_expiry_check CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS auth_sessions_user_id_idx ON auth_sessions (user_id);
CREATE INDEX IF NOT EXISTS auth_sessions_expires_at_idx ON auth_sessions (expires_at);
