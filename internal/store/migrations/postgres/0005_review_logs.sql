-- 0005: 审查执行过程日志（实时进度，前端轮询展示）

CREATE TABLE IF NOT EXISTS review_logs (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    review_id  BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    level      TEXT NOT NULL DEFAULT 'info',  -- info|warn|error
    message    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_review_logs_review ON review_logs(review_id, id);
