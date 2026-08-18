-- 0003: 可复用凭据库（SSH 密钥对 / HTTPS Token）+ 仓库绑定 (MySQL)

CREATE TABLE IF NOT EXISTS credentials (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    `name`       VARCHAR(255) NOT NULL,
    type         VARCHAR(32) NOT NULL,            -- ssh | https_token
    secret       TEXT NOT NULL,                   -- SSH 私钥 PEM / HTTPS token（不通过 API 返回明文）
    public_key   TEXT NOT NULL,                   -- ssh: OpenSSH 公钥行
    fingerprint  VARCHAR(255) NOT NULL DEFAULT '', -- ssh: SHA256 指纹
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 仓库绑定的凭据；删除凭据时自动解绑（ON DELETE SET NULL）。
-- MySQL 原生不支持 ADD COLUMN IF NOT EXISTS，用 information_schema + 预处理保证幂等。
SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'repos' AND COLUMN_NAME = 'credential_id'
);
SET @col_sql := IF(@col_exists = 0,
    'ALTER TABLE repos ADD COLUMN credential_id BIGINT NULL',
    'SELECT 1');
PREPARE stmt FROM @col_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 幂等添加外键（MySQL 无 ADD CONSTRAINT IF NOT EXISTS）。
SET @fk_exists := (
    SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS
    WHERE CONSTRAINT_SCHEMA = DATABASE()
      AND TABLE_NAME = 'repos' AND CONSTRAINT_NAME = 'fk_repos_credential'
);
SET @fk_sql := IF(@fk_exists = 0,
    'ALTER TABLE repos ADD CONSTRAINT fk_repos_credential FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE SET NULL',
    'SELECT 1');
PREPARE stmt FROM @fk_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
