-- Node.js for deploy scripts. node_version is "auto" (read .nvmrc,
-- .node-version or package.json, else the default version), "none", or a
-- version spec such as "22" or "lts". node_used is the version the latest
-- deployment ran with.
ALTER TABLE projects ADD COLUMN node_version TEXT NOT NULL DEFAULT 'auto';
ALTER TABLE projects ADD COLUMN node_used TEXT NOT NULL DEFAULT '';

-- Server-wide settings kept in the database, such as the default Node.js version.
CREATE TABLE settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  DATETIME NOT NULL
);
