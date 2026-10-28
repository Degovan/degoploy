-- Environment variables passed to every deploy script of a project. Secret
-- values are stored sealed (see internal/secrets) and never returned by the API.
CREATE TABLE project_variables (
    id          INTEGER PRIMARY KEY,
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    value       TEXT NOT NULL,
    secret      INTEGER NOT NULL DEFAULT 0,
    updated_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    UNIQUE (project_id, name)
);
