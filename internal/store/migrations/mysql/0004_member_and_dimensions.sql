-- 0004: 成员备注（authors）、凭据平台字段、审查维度 JSON (MySQL)

CREATE TABLE IF NOT EXISTS authors (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    git_login    VARCHAR(255) NOT NULL UNIQUE,  -- 与 reviews.author 一致的平台 login
    display_name VARCHAR(255) NOT NULL DEFAULT '', -- 真实姓名
    team         VARCHAR(255) NOT NULL DEFAULT '',
    note         VARCHAR(1024) NOT NULL DEFAULT '',
    active       TINYINT NOT NULL DEFAULT 1,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 凭据可用于平台 API（批量导入 / 解析分支 HEAD）
SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'credentials' AND COLUMN_NAME = 'provider'
);
SET @col_sql := IF(@col_exists = 0,
    'ALTER TABLE credentials ADD COLUMN provider VARCHAR(32) NOT NULL DEFAULT ''''',
    'SELECT 1');
PREPARE stmt FROM @col_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'credentials' AND COLUMN_NAME = 'api_base_url'
);
SET @col_sql := IF(@col_exists = 0,
    'ALTER TABLE credentials ADD COLUMN api_base_url VARCHAR(1024) NOT NULL DEFAULT ''''',
    'SELECT 1');
PREPARE stmt FROM @col_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 审查维度评分：{"dim_key":{"score":80,"rationale":"...","label":"..."}}
SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'reviews' AND COLUMN_NAME = 'score_dimensions'
);
SET @col_sql := IF(@col_exists = 0,
    'ALTER TABLE reviews ADD COLUMN score_dimensions TEXT NOT NULL',
    'SELECT 1');
PREPARE stmt FROM @col_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
