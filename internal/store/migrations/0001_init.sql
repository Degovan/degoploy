CREATE TABLE users (
    id             INTEGER PRIMARY KEY,
    username       TEXT NOT NULL UNIQUE COLLATE NOCASE,
    email          TEXT NOT NULL DEFAULT '',
    display_name   TEXT NOT NULL DEFAULT '',
    auth_provider  TEXT NOT NULL CHECK (auth_provider IN ('local', 'oidc')),
    oidc_issuer    TEXT,
    oidc_subject   TEXT,
    password_hash  TEXT,
    role           TEXT NOT NULL CHECK (role IN ('admin', 'deployer', 'viewer')),
    disabled       INTEGER NOT NULL DEFAULT 0,
    last_login_at  DATETIME,
    created_at     DATETIME NOT NULL,
    UNIQUE (oidc_issuer, oidc_subject)
);

CREATE TABLE sessions (
    id_hash     TEXT PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token  TEXT NOT NULL,
    expires_at  DATETIME NOT NULL,
    created_at  DATETIME NOT NULL,
    user_agent  TEXT NOT NULL DEFAULT '',
    ip          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user_id ON sessions(user_id);
CREATE INDEX sessions_expires_at ON sessions(expires_at);

CREATE TABLE projects (
    id              INTEGER PRIMARY KEY,
    public_id       TEXT NOT NULL UNIQUE,
    slug            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    provider        TEXT NOT NULL CHECK (provider IN ('github', 'gitea')),
    repo_url        TEXT NOT NULL,
    work_dir        TEXT NOT NULL,
    script_source   TEXT NOT NULL DEFAULT 'managed' CHECK (script_source IN ('managed', 'repo')),
    script_path     TEXT NOT NULL DEFAULT '',
    webhook_secret  TEXT NOT NULL,
    auto_pull       INTEGER NOT NULL DEFAULT 1,
    timeout_sec     INTEGER NOT NULL DEFAULT 600,
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    CHECK (script_source = 'managed' OR auto_pull = 1)
);

CREATE TABLE project_members (
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK (role IN ('admin', 'deployer', 'viewer')),
    PRIMARY KEY (project_id, user_id)
);

CREATE TABLE trigger_rules (
    id             INTEGER PRIMARY KEY,
    project_id     INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    enabled        INTEGER NOT NULL DEFAULT 1,
    position       INTEGER NOT NULL DEFAULT 0,
    event          TEXT NOT NULL CHECK (event IN ('push', 'tag')),
    ref_pattern    TEXT NOT NULL,
    paths_include  TEXT NOT NULL DEFAULT '[]',
    paths_exclude  TEXT NOT NULL DEFAULT '[]',
    skip_keyword   TEXT NOT NULL DEFAULT '[skip deploy]',
    extra_env      TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX trigger_rules_project ON trigger_rules(project_id, position);

CREATE TABLE script_revisions (
    id          INTEGER PRIMARY KEY,
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    content     TEXT NOT NULL,
    created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at  DATETIME NOT NULL
);
CREATE INDEX script_revisions_project ON script_revisions(project_id, id DESC);

CREATE TABLE deployments (
    id              INTEGER PRIMARY KEY,
    project_id      INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    rule_id         INTEGER REFERENCES trigger_rules(id) ON DELETE SET NULL,
    trigger         TEXT NOT NULL CHECK (trigger IN ('webhook', 'manual', 'redeploy')),
    triggered_by    INTEGER REFERENCES users(id) ON DELETE SET NULL,
    ref             TEXT NOT NULL DEFAULT '',
    commit_sha      TEXT NOT NULL DEFAULT '',
    commit_message  TEXT NOT NULL DEFAULT '',
    author          TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL CHECK (status IN ('queued', 'running', 'success', 'failed', 'canceled', 'timeout', 'skipped')),
    exit_code       INTEGER,
    log_path        TEXT NOT NULL DEFAULT '',
    queued_at       DATETIME NOT NULL,
    started_at      DATETIME,
    finished_at     DATETIME
);
CREATE INDEX deployments_project ON deployments(project_id, id DESC);
CREATE INDEX deployments_status ON deployments(status);

CREATE TABLE webhook_events (
    id             INTEGER PRIMARY KEY,
    project_id     INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    provider       TEXT NOT NULL,
    event          TEXT NOT NULL,
    delivery_id    TEXT NOT NULL DEFAULT '',
    ref            TEXT NOT NULL DEFAULT '',
    commit_sha     TEXT NOT NULL DEFAULT '',
    matched        INTEGER NOT NULL,
    reason         TEXT NOT NULL DEFAULT '',
    deployment_id  INTEGER REFERENCES deployments(id) ON DELETE SET NULL,
    received_at    DATETIME NOT NULL
);
CREATE INDEX webhook_events_project ON webhook_events(project_id, id DESC);

CREATE TABLE audit_logs (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER REFERENCES users(id) ON DELETE SET NULL,
    action       TEXT NOT NULL,
    target_type  TEXT NOT NULL DEFAULT '',
    target_id    TEXT NOT NULL DEFAULT '',
    detail       TEXT NOT NULL DEFAULT '{}',
    ip           TEXT NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL
);
CREATE INDEX audit_logs_created ON audit_logs(id DESC);
