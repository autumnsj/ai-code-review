-- 0007: 日报/周报落库（MySQL）。报告每次发送（定时/手动）时把标题与 markdown
-- 正文存入 reports 表，平台「报告记录」页可回看（设置页预览不落库）。
-- job_id 唯一：同一 job 失败重试不会产生重复报告。
-- 索引内联在 CREATE TABLE 中（MySQL 的 CREATE INDEX 无 IF NOT EXISTS，随表创建才幂等）；
-- content 为 TEXT 列，MySQL 不允许 DEFAULT，INSERT 时显式传值。
CREATE TABLE IF NOT EXISTS reports (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    kind          VARCHAR(16) NOT NULL DEFAULT '',
    trigger_type  VARCHAR(16) NOT NULL DEFAULT '',
    period_start  DATETIME NOT NULL,
    period_end    DATETIME NOT NULL,
    title         VARCHAR(255) NOT NULL DEFAULT '',
    content       TEXT NOT NULL,
    job_id        BIGINT NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_reports_created (created_at),
    UNIQUE KEY uq_reports_job (job_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
