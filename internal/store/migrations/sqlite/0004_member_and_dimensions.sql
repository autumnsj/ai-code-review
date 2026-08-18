-- 0004: 成员备注（authors）、凭据平台字段、审查维度 JSON

CREATE TABLE IF NOT EXISTS authors (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    git_login    TEXT NOT NULL UNIQUE,          -- 与 reviews.author 一致的平台 login
    display_name TEXT NOT NULL DEFAULT '',       -- 真实姓名
    team         TEXT NOT NULL DEFAULT '',
    note         TEXT NOT NULL DEFAULT '',
    active       INTEGER NOT NULL DEFAULT 1,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- 凭据可用于平台 API（批量导入 / 解析分支 HEAD）
ALTER TABLE credentials ADD COLUMN provider TEXT NOT NULL DEFAULT '';
ALTER TABLE credentials ADD COLUMN api_base_url TEXT NOT NULL DEFAULT '';

-- 审查维度评分：{"dim_key":{"score":80,"rationale":"...","label":"..."}}
ALTER TABLE reviews ADD COLUMN score_dimensions TEXT NOT NULL DEFAULT '{}';
