-- Releases: with releases on, each deployment checks out into
-- <work_dir>/releases/<deployment id> and <work_dir>/current switches to it
-- when the script succeeds. shared_paths (a JSON array) are linked from
-- <work_dir>/shared into every release; after_activate runs after each switch.
ALTER TABLE projects ADD COLUMN releases INTEGER NOT NULL DEFAULT 0;
ALTER TABLE projects ADD COLUMN keep_releases INTEGER NOT NULL DEFAULT 5;
ALTER TABLE projects ADD COLUMN shared_paths TEXT NOT NULL DEFAULT '[]';
ALTER TABLE projects ADD COLUMN after_activate TEXT NOT NULL DEFAULT '';

-- Deployments gain the "rollback" trigger and release_id: the release a
-- deployment built, or switched back to. SQLite cannot change a CHECK, so the
-- table is rebuilt. Dropping the old table clears webhook_events.deployment_id
-- (ON DELETE SET NULL), so those links are kept aside and put back.
CREATE TEMP TABLE webhook_deployments AS
    SELECT id, deployment_id FROM webhook_events WHERE deployment_id IS NOT NULL;

CREATE TABLE deployments_new (
    id                  INTEGER PRIMARY KEY,
    project_id          INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    rule_id             INTEGER REFERENCES trigger_rules(id) ON DELETE SET NULL,
    trigger             TEXT NOT NULL CHECK (trigger IN ('webhook', 'manual', 'redeploy', 'rollback')),
    triggered_by        INTEGER REFERENCES users(id) ON DELETE SET NULL,
    ref                 TEXT NOT NULL DEFAULT '',
    commit_sha          TEXT NOT NULL DEFAULT '',
    commit_message      TEXT NOT NULL DEFAULT '',
    author              TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL CHECK (status IN ('queued', 'running', 'success', 'failed', 'canceled', 'timeout', 'skipped')),
    exit_code           INTEGER,
    log_path            TEXT NOT NULL DEFAULT '',
    queued_at           DATETIME NOT NULL,
    started_at          DATETIME,
    finished_at         DATETIME,
    script_started_at   DATETIME,
    error               TEXT NOT NULL DEFAULT '',
    auto_pull           INTEGER,
    script_revision_id  INTEGER REFERENCES script_revisions(id) ON DELETE SET NULL,
    release_id          INTEGER
);

INSERT INTO deployments_new (id, project_id, rule_id, trigger, triggered_by, ref, commit_sha, commit_message,
    author, status, exit_code, log_path, queued_at, started_at, finished_at, script_started_at, error, auto_pull,
    script_revision_id)
SELECT id, project_id, rule_id, trigger, triggered_by, ref, commit_sha, commit_message,
    author, status, exit_code, log_path, queued_at, started_at, finished_at, script_started_at, error, auto_pull,
    script_revision_id
FROM deployments;

DROP TABLE deployments;
ALTER TABLE deployments_new RENAME TO deployments;
CREATE INDEX deployments_project ON deployments(project_id, id DESC);
CREATE INDEX deployments_status ON deployments(status);

UPDATE webhook_events SET deployment_id = (
    SELECT w.deployment_id FROM temp.webhook_deployments w WHERE w.id = webhook_events.id
) WHERE id IN (SELECT id FROM temp.webhook_deployments);
DROP TABLE temp.webhook_deployments;
