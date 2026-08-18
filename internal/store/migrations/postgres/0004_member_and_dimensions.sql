-- 0004: 成员备注（authors）、凭据平台字段、审查维度 JSON

CREATE TABLE IF NOT EXISTS authors (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    git_login    TEXT NOT NULL UNIQUE,          -- 与 reviews.author 一致的平台 login
    display_name TEXT NOT NULL DEFAULT '',       -- 真实姓名
    team         TEXT NOT NULL DEFAULT '',
    note         TEXT NOT NULL DEFAULT '',
    active       SMALLINT NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 凭据可用于平台 API（批量导入 / 解析分支 HEAD）
ALTER TABLE credentials ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT '';
ALTER TABLE credentials ADD COLUMN IF NOT EXISTS api_base_url TEXT NOT NULL DEFAULT '';

-- 审查维度评分：{"dim_key":{"score":80,"rationale":"...","label":"..."}}
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS score_dimensions TEXT NOT NULL DEFAULT '{}';
