-- 0008: 报告以人为单位。reports 增加 author 列（个人报告存成员展示名，空串 = 团队简报/异常提醒），
-- 唯一约束从 job_id 改为 (job_id, author)：一个 report 任务现在为每位成员各产生一条记录。
ALTER TABLE reports ADD COLUMN author TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS uq_reports_job;
CREATE UNIQUE INDEX IF NOT EXISTS uq_reports_job_author ON reports(job_id, author);
CREATE INDEX IF NOT EXISTS idx_reports_author ON reports(author);
