-- 0009: 功能数改为 AI 看代码判断（MySQL）。review_author_reports 增加 feature_count：
-- 该作者在本次审查区间内被 AI 归属的功能/工作项数量。0 = 历史数据，统计时按 1 回退。
-- MySQL 的 ALTER ADD COLUMN 无 IF NOT EXISTS，用 information_schema + PREPARE 保证幂等。
SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'review_author_reports' AND COLUMN_NAME = 'feature_count'
);
SET @col_sql := IF(@col_exists = 0,
    'ALTER TABLE review_author_reports ADD COLUMN feature_count INT NOT NULL DEFAULT 0',
    'SELECT 1');
PREPARE stmt FROM @col_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
