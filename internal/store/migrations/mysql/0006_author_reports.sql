-- 0006: 多作者归属（MySQL）。AI 只审一次，git blame 归属每条 finding 到作者，
-- 并在 review_author_reports 为每位参与者生成独立报告。

-- findings 记录归属作者（blame 得到的 commit author email；无法归属时为空串）。
SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'findings' AND COLUMN_NAME = 'author'
);
SET @col_sql := IF(@col_exists = 0,
    'ALTER TABLE findings ADD COLUMN author VARCHAR(191) NOT NULL DEFAULT ''''',
    'SELECT 1');
PREPARE stmt FROM @col_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*) FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'findings' AND INDEX_NAME = 'idx_findings_review_author'
);
SET @idx_sql := IF(@idx_exists = 0,
    'ALTER TABLE findings ADD INDEX idx_findings_review_author (review_id, author)',
    'SELECT 1');
PREPARE stmt FROM @idx_sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

CREATE TABLE IF NOT EXISTS review_author_reports (
    id               BIGINT AUTO_INCREMENT PRIMARY KEY,
    review_id        BIGINT NOT NULL,
    author           VARCHAR(191) NOT NULL DEFAULT '',
    author_name      VARCHAR(255) NOT NULL DEFAULT '',
    public_token     VARCHAR(191) NOT NULL,
    summary          TEXT NOT NULL,
    score_total      INT NOT NULL DEFAULT 0,
    score_arch       INT NOT NULL DEFAULT 0,
    score_quality    INT NOT NULL DEFAULT 0,
    score_security   INT NOT NULL DEFAULT 0,
    score_maint      INT NOT NULL DEFAULT 0,
    score_dimensions TEXT NOT NULL,
    findings_count   INT NOT NULL DEFAULT 0,
    critical_count   INT NOT NULL DEFAULT 0,
    high_count       INT NOT NULL DEFAULT 0,
    medium_count     INT NOT NULL DEFAULT 0,
    low_count        INT NOT NULL DEFAULT 0,
    info_count       INT NOT NULL DEFAULT 0,
    additions        INT NOT NULL DEFAULT 0,
    deletions        INT NOT NULL DEFAULT 0,
    files_changed    INT NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_author_reports_token (public_token),
    UNIQUE KEY uk_author_reports_review_author (review_id, author),
    KEY idx_author_reports_author (author),
    CONSTRAINT fk_author_reports_review FOREIGN KEY (review_id) REFERENCES reviews(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 回填：为历史已成功的单作者审查补一条作者报告，复用 review.public_token。
INSERT INTO review_author_reports
  (review_id, author, author_name, public_token, summary,
   score_total, score_arch, score_quality, score_security, score_maint, score_dimensions,
   findings_count, critical_count, high_count, medium_count, low_count, info_count,
   additions, deletions, files_changed, created_at)
SELECT rv.id, rv.author, '', rv.public_token, rv.summary,
  rv.score_total, rv.score_arch, rv.score_quality, rv.score_security, rv.score_maint,
  CASE WHEN rv.score_dimensions='' THEN '{}' ELSE rv.score_dimensions END,
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id),
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id AND f.severity='critical'),
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id AND f.severity='high'),
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id AND f.severity='medium'),
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id AND f.severity='low'),
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id AND f.severity='info'),
  rv.additions, rv.deletions, rv.files_changed, COALESCE(rv.finished_at, rv.triggered_at)
FROM reviews rv
LEFT JOIN review_author_reports ex ON ex.review_id = rv.id
WHERE rv.status='succeeded' AND rv.author<>'' AND ex.id IS NULL;
