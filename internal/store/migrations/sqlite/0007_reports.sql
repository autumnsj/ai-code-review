-- 0007: 日报/周报落库。报告每次发送（定时/手动）时把标题与 markdown 正文
-- 存入 reports 表，平台「报告记录」页可回看（设置页预览不落库）。
-- job_id 唯一：同一 job 失败重试不会产生重复报告。
CREATE TABLE IF NOT EXISTS reports (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    kind          TEXT NOT NULL DEFAULT '',        -- daily | weekly
    trigger_type  TEXT NOT NULL DEFAULT '',        -- scheduled | manual
    period_start  DATETIME NOT NULL,
    period_end    DATETIME NOT NULL,
    title         TEXT NOT NULL DEFAULT '',
    content       TEXT NOT NULL,
    job_id        INTEGER NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_reports_created ON reports(created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_reports_job ON reports(job_id);
