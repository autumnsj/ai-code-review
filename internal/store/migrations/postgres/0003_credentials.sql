-- 0003: 可复用凭据库（SSH 密钥对 / HTTPS Token）+ 仓库绑定

CREATE TABLE IF NOT EXISTS credentials (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         TEXT NOT NULL,
    type         TEXT NOT NULL,            -- ssh | https_token
    secret       TEXT NOT NULL DEFAULT '', -- SSH 私钥 PEM / HTTPS token（不通过 API 返回明文）
    public_key   TEXT NOT NULL DEFAULT '', -- ssh: OpenSSH 公钥行
    fingerprint  TEXT NOT NULL DEFAULT '', -- ssh: SHA256 指纹
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE repos ADD COLUMN IF NOT EXISTS credential_id BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'repos_credential_id_fkey'
    ) THEN
        ALTER TABLE repos ADD CONSTRAINT repos_credential_id_fkey
            FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE SET NULL;
    END IF;
END $$;
