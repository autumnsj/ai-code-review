-- 0001_init.sql 初始 schema (MySQL / MariaDB 方言)
-- 说明：需要在 DSN 中开启 multiStatements=true（迁移框架一次执行整个文件）。
-- 标识/URL 类用 VARCHAR(191) 以兼容 utf8mb4 索引长度上限；
-- 长文本列（findings/credentials 等）用 TEXT，且所有 INSERT 均显式传值，不依赖默认值。

CREATE TABLE IF NOT EXISTS settings (
    `key`       VARCHAR(191) PRIMARY KEY,
    `value`     TEXT NOT NULL,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS repos (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    provider        VARCHAR(32) NOT NULL,
    clone_url       VARCHAR(1024) NOT NULL,
    web_url         VARCHAR(1024) NOT NULL DEFAULT '',
    `name`          VARCHAR(255) NOT NULL,
    default_branch  VARCHAR(128) NOT NULL DEFAULT 'main',
    access_token    VARCHAR(1024) NOT NULL DEFAULT '',
    hook_token      VARCHAR(191) NOT NULL UNIQUE,
    hook_secret     VARCHAR(512) NOT NULL DEFAULT '',
    status          VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS reviews (
    id             BIGINT AUTO_INCREMENT PRIMARY KEY,
    repo_id        BIGINT NOT NULL,
    public_token   VARCHAR(191) NOT NULL UNIQUE,
    commit_sha     VARCHAR(64) NOT NULL,
    base_sha       VARCHAR(64) NOT NULL DEFAULT '',
    target_ref     VARCHAR(255) NOT NULL DEFAULT '',
    source_ref     VARCHAR(255) NOT NULL DEFAULT '',
    pr_number      BIGINT NOT NULL DEFAULT 0,
    pr_title       VARCHAR(512) NOT NULL DEFAULT '',
    pr_url         VARCHAR(1024) NOT NULL DEFAULT '',
    author         VARCHAR(255) NOT NULL DEFAULT '',
    event_type     VARCHAR(32) NOT NULL DEFAULT 'push',
    status         VARCHAR(32) NOT NULL DEFAULT 'pending',
    summary        TEXT NOT NULL,
    score_total    INT NOT NULL DEFAULT 0,
    score_arch     INT NOT NULL DEFAULT 0,
    score_quality  INT NOT NULL DEFAULT 0,
    score_security INT NOT NULL DEFAULT 0,
    score_maint    INT NOT NULL DEFAULT 0,
    stats          TEXT NOT NULL,
    diff_truncated TINYINT NOT NULL DEFAULT 0,
    error          TEXT NOT NULL,
    tokens_used    BIGINT NOT NULL DEFAULT 0,
    additions      BIGINT NOT NULL DEFAULT 0,
    deletions      BIGINT NOT NULL DEFAULT 0,
    files_changed  INT NOT NULL DEFAULT 0,
    triggered_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at     DATETIME NULL,
    finished_at    DATETIME NULL,
    UNIQUE KEY uq_repo_commit_ref (repo_id, commit_sha, target_ref(191)),
    KEY idx_reviews_repo_finished (repo_id, finished_at),
    KEY idx_reviews_author (author),
    CONSTRAINT fk_reviews_repo FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS findings (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    review_id   BIGINT NOT NULL,
    source      VARCHAR(64) NOT NULL,
    rule_id     VARCHAR(128) NOT NULL DEFAULT '',
    severity    VARCHAR(32) NOT NULL,
    category    VARCHAR(64) NOT NULL DEFAULT '',
    file_path   VARCHAR(512) NOT NULL DEFAULT '',
    line_start  INT NOT NULL DEFAULT 0,
    line_end    INT NOT NULL DEFAULT 0,
    title       VARCHAR(512) NOT NULL,
    message     TEXT NOT NULL,
    snippet     TEXT NOT NULL,
    suggestion  TEXT NOT NULL,
    confidence  VARCHAR(32) NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_findings_review_severity (review_id, severity),
    KEY idx_findings_review_file (review_id, file_path(191)),
    CONSTRAINT fk_findings_review FOREIGN KEY (review_id) REFERENCES reviews(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS review_configs (
    repo_id              BIGINT PRIMARY KEY,
    enabled_checkers     VARCHAR(2048) NOT NULL DEFAULT '[]',
    enabled_dimensions   VARCHAR(2048) NOT NULL DEFAULT '[]',
    ignore_paths         VARCHAR(2048) NOT NULL DEFAULT '[]',
    custom_prompt        TEXT NOT NULL,
    score_weights        VARCHAR(1024) NOT NULL DEFAULT '{}',
    notify_on            VARCHAR(255) NOT NULL DEFAULT '["success"]',
    min_severity         VARCHAR(32) NOT NULL DEFAULT 'medium',
    llm_model_override   VARCHAR(255) NOT NULL DEFAULT '',
    updated_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_review_configs_repo FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS notifications (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    repo_id       BIGINT NOT NULL,
    review_id     BIGINT NOT NULL,
    channel       VARCHAR(32) NOT NULL,
    webhook_url   VARCHAR(1024) NOT NULL,
    secret        VARCHAR(512) NOT NULL DEFAULT '',
    status        VARCHAR(32) NOT NULL DEFAULT 'pending',
    response_code INT NOT NULL DEFAULT 0,
    error         VARCHAR(2048) NOT NULL DEFAULT '',
    sent_at       DATETIME NULL,
    KEY idx_notifications_review (review_id),
    CONSTRAINT fk_notifications_repo FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE,
    CONSTRAINT fk_notifications_review FOREIGN KEY (review_id) REFERENCES reviews(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS jobs (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    kind            VARCHAR(64) NOT NULL,
    payload         TEXT NOT NULL,
    status          VARCHAR(32) NOT NULL DEFAULT 'pending',
    lease_owner     VARCHAR(191) NOT NULL DEFAULT '',
    lease_until     DATETIME NULL,
    attempts        INT NOT NULL DEFAULT 0,
    max_attempts    INT NOT NULL DEFAULT 3,
    last_error      VARCHAR(2048) NOT NULL DEFAULT '',
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    available_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at      DATETIME NULL,
    finished_at     DATETIME NULL,
    KEY idx_jobs_status (status, lease_until)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
