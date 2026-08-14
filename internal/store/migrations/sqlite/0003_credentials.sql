-- 0003: 可复用凭据库（SSH 密钥对 / HTTPS Token）+ 仓库绑定

CREATE TABLE IF NOT EXISTS credentials (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL,
    type         TEXT NOT NULL,            -- ssh | https_token
    secret       TEXT NOT NULL DEFAULT '', -- SSH 私钥 PEM / HTTPS token（不通过 API 返回明文）
    public_key   TEXT NOT NULL DEFAULT '', -- ssh: OpenSSH 公钥行
    fingerprint  TEXT NOT NULL DEFAULT '', -- ssh: SHA256 指纹
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- 仓库绑定的凭据；删除凭据时自动解绑（ON DELETE SET NULL）。
-- 列必须可空且默认 NULL，以满足 SQLite ADD COLUMN 外键约束。
ALTER TABLE repos ADD COLUMN credential_id INTEGER REFERENCES credentials(id) ON DELETE SET NULL;
