-- 0006: 多作者归属。AI 只审一次，git blame 归属每条 finding 到作者，
-- 并在 review_author_reports 为每位参与者生成独立报告。

ALTER TABLE findings ADD COLUMN IF NOT EXISTS author TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS review_author_reports (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    review_id       BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    author          TEXT NOT NULL DEFAULT '',
    author_name     TEXT NOT NULL DEFAULT '',
    public_token    TEXT NOT NULL UNIQUE,
    summary         TEXT NOT NULL DEFAULT '',
    score_total     INTEGER NOT NULL DEFAULT 0,
    score_arch      INTEGER NOT NULL DEFAULT 0,
    score_quality   INTEGER NOT NULL DEFAULT 0,
    score_security  INTEGER NOT NULL DEFAULT 0,
    score_maint     INTEGER NOT NULL DEFAULT 0,
    score_dimensions TEXT NOT NULL DEFAULT '{}',
    findings_count  INTEGER NOT NULL DEFAULT 0,
    critical_count  INTEGER NOT NULL DEFAULT 0,
    high_count      INTEGER NOT NULL DEFAULT 0,
    medium_count    INTEGER NOT NULL DEFAULT 0,
    low_count       INTEGER NOT NULL DEFAULT 0,
    info_count      INTEGER NOT NULL DEFAULT 0,
    additions       INTEGER NOT NULL DEFAULT 0,
    deletions       INTEGER NOT NULL DEFAULT 0,
    files_changed   INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_author_reports_review_author ON review_author_reports(review_id, author);
CREATE INDEX IF NOT EXISTS idx_author_reports_author ON review_author_reports(author);

-- 回填：为历史已成功的单作者审查补一条作者报告，复用 review.public_token。
INSERT INTO review_author_reports
  (review_id, author, author_name, public_token, summary,
   score_total, score_arch, score_quality, score_security, score_maint, score_dimensions,
   findings_count, critical_count, high_count, medium_count, low_count, info_count,
   additions, deletions, files_changed, created_at)
SELECT rv.id, rv.author, '', rv.public_token, rv.summary,
  rv.score_total, rv.score_arch, rv.score_quality, rv.score_security, rv.score_maint,
  CASE WHEN rv.score_dimensions='{}' THEN '{}' ELSE rv.score_dimensions END,
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id),
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id AND f.severity='critical'),
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id AND f.severity='high'),
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id AND f.severity='medium'),
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id AND f.severity='low'),
  (SELECT COUNT(*) FROM findings f WHERE f.review_id=rv.id AND f.severity='info'),
  rv.additions, rv.deletions, rv.files_changed, COALESCE(rv.finished_at, rv.triggered_at)
FROM reviews rv
WHERE rv.status='succeeded' AND rv.author<>''
  AND NOT EXISTS (SELECT 1 FROM review_author_reports ex WHERE ex.review_id=rv.id);
