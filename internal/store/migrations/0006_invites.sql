-- Pending invitations to create a local account. Only the hash of the link's
-- token is stored; accepting an invite creates the user and deletes the row.
CREATE TABLE invites (
    id          INTEGER PRIMARY KEY,
    email       TEXT NOT NULL UNIQUE COLLATE NOCASE,
    role        TEXT NOT NULL CHECK (role IN ('admin', 'deployer', 'viewer')),
    token_hash  TEXT NOT NULL UNIQUE,
    invited_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at  DATETIME NOT NULL,
    expires_at  DATETIME NOT NULL
);
