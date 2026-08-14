-- 0001_init.sql 初始 schema

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS repos (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    provider        TEXT NOT NULL,
    clone_url       TEXT NOT NULL,
    web_url         TEXT NOT NULL DEFAULT '',
    name            TEXT NOT NULL,
    default_branch  TEXT NOT NULL DEFAULT 'main',
    access_token    TEXT NOT NULL DEFAULT '',
    hook_token      TEXT NOT NULL UNIQUE,
    hook_secret     TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS reviews (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id        INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    public_token   TEXT NOT NULL UNIQUE,
    commit_sha     TEXT NOT NULL,
    base_sha       TEXT NOT NULL DEFAULT '',
    target_ref     TEXT NOT NULL DEFAULT '',
    source_ref     TEXT NOT NULL DEFAULT '',
    pr_number      INTEGER NOT NULL DEFAULT 0,
    pr_title       TEXT NOT NULL DEFAULT '',
    pr_url         TEXT NOT NULL DEFAULT '',
    author         TEXT NOT NULL DEFAULT '',
    event_type     TEXT NOT NULL DEFAULT 'push',
    status         TEXT NOT NULL DEFAULT 'pending',
    summary        TEXT NOT NULL DEFAULT '',
    score_total    INTEGER NOT NULL DEFAULT 0,
    score_arch     INTEGER NOT NULL DEFAULT 0,
    score_quality  INTEGER NOT NULL DEFAULT 0,
    score_security INTEGER NOT NULL DEFAULT 0,
    score_maint    INTEGER NOT NULL DEFAULT 0,
    stats          TEXT NOT NULL DEFAULT '',
    diff_truncated INTEGER NOT NULL DEFAULT 0,
    error          TEXT NOT NULL DEFAULT '',
    tokens_used    INTEGER NOT NULL DEFAULT 0,
    triggered_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    started_at     DATETIME,
    finished_at    DATETIME,
    UNIQUE(repo_id, commit_sha, target_ref)
);
CREATE INDEX IF NOT EXISTS idx_reviews_repo_finished ON reviews(repo_id, finished_at DESC);

CREATE TABLE IF NOT EXISTS findings (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    review_id   INTEGER NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    source      TEXT NOT NULL,
    rule_id     TEXT NOT NULL DEFAULT '',
    severity    TEXT NOT NULL,
    category    TEXT NOT NULL DEFAULT '',
    file_path   TEXT NOT NULL DEFAULT '',
    line_start  INTEGER NOT NULL DEFAULT 0,
    line_end    INTEGER NOT NULL DEFAULT 0,
    title       TEXT NOT NULL,
    message     TEXT NOT NULL DEFAULT '',
    snippet     TEXT NOT NULL DEFAULT '',
    suggestion  TEXT NOT NULL DEFAULT '',
    confidence  TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_findings_review_severity ON findings(review_id, severity);
CREATE INDEX IF NOT EXISTS idx_findings_review_file ON findings(review_id, file_path);

CREATE TABLE IF NOT EXISTS review_configs (
    repo_id              INTEGER PRIMARY KEY REFERENCES repos(id) ON DELETE CASCADE,
    enabled_checkers     TEXT NOT NULL DEFAULT '[]',
    enabled_dimensions   TEXT NOT NULL DEFAULT '[]',
    ignore_paths         TEXT NOT NULL DEFAULT '[]',
    custom_prompt        TEXT NOT NULL DEFAULT '',
    score_weights        TEXT NOT NULL DEFAULT '{}',
    notify_on            TEXT NOT NULL DEFAULT '["success"]',
    min_severity         TEXT NOT NULL DEFAULT 'medium',
    llm_model_override   TEXT NOT NULL DEFAULT '',
    updated_at           DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS notifications (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id       INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    review_id     INTEGER NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    channel       TEXT NOT NULL,
    webhook_url   TEXT NOT NULL,
    secret        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'pending',
    response_code INTEGER NOT NULL DEFAULT 0,
    error         TEXT NOT NULL DEFAULT '',
    sent_at       DATETIME
);
CREATE INDEX IF NOT EXISTS idx_notifications_review ON notifications(review_id);

CREATE TABLE IF NOT EXISTS jobs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    kind            TEXT NOT NULL,
    payload         TEXT NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'pending',
    lease_owner     TEXT NOT NULL DEFAULT '',
    lease_until     DATETIME,
    attempts        INTEGER NOT NULL DEFAULT 0,
    max_attempts    INTEGER NOT NULL DEFAULT 3,
    last_error      TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL UNIQUE,
    available_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    started_at      DATETIME,
    finished_at     DATETIME
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status, lease_until);
