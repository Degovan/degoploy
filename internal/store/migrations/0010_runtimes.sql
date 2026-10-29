-- Runtimes (Node.js, PHP) per project. spec is "auto" (use the runtime when
-- the repository does), "off", or a version spec such as "22" or "8.3"; used
-- is the version the latest deployment ran with. A project without a row
-- for a runtime is on "auto".
CREATE TABLE project_runtimes (
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    runtime     TEXT NOT NULL,
    spec        TEXT NOT NULL,
    used        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (project_id, runtime)
);

INSERT INTO project_runtimes (project_id, runtime, spec, used)
SELECT id, 'node', CASE node_version WHEN 'none' THEN 'off' ELSE node_version END, node_used
FROM projects;

ALTER TABLE projects DROP COLUMN node_version;
ALTER TABLE projects DROP COLUMN node_used;

UPDATE settings SET key = 'runtime.node.default' WHERE key = 'node.default';
