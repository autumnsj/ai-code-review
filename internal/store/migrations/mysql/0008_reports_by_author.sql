-- 0008: 报告以人为单位（MySQL）。reports 增加 author 列（个人报告存成员展示名，空串 = 团队简报/异常），
-- 唯一约束从 job_id 改为 (job_id, author)：一个 report 任务现在为每位成员各产生一条记录。
-- MySQL 的 ALTER/ADD INDEX 无 IF NOT EXISTS，全部用 information_schema + PREPARE 保证幂等。

-- 加列 author
SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'reports' AND COLUMN_NAME = 'author'
);
SET @col_sql := IF(@col_exists = 0,
    'ALTER TABLE reports ADD COLUMN author VARCHAR(191) NOT NULL DEFAULT ''''',
    'SELECT 1');
PREPARE stmt FROM @col_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 删旧唯一索引 uq_reports_job
SET @old_exists := (
    SELECT COUNT(*) FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'reports' AND INDEX_NAME = 'uq_reports_job'
);
SET @old_sql := IF(@old_exists > 0,
    'ALTER TABLE reports DROP INDEX uq_reports_job',
    'SELECT 1');
PREPARE stmt FROM @old_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 加新唯一索引 (job_id, author)
SET @uq_exists := (
    SELECT COUNT(*) FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'reports' AND INDEX_NAME = 'uq_reports_job_author'
);
SET @uq_sql := IF(@uq_exists = 0,
    'ALTER TABLE reports ADD UNIQUE KEY uq_reports_job_author (job_id, author)',
    'SELECT 1');
PREPARE stmt FROM @uq_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 加普通索引 author
SET @ai_exists := (
    SELECT COUNT(*) FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'reports' AND INDEX_NAME = 'idx_reports_author'
);
SET @ai_sql := IF(@ai_exists = 0,
    'ALTER TABLE reports ADD KEY idx_reports_author (author)',
    'SELECT 1');
PREPARE stmt FROM @ai_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
