-- 0002: 作者维度统计所需的去规范化列与索引（SQLite）
ALTER TABLE reviews ADD COLUMN additions INTEGER NOT NULL DEFAULT 0;
ALTER TABLE reviews ADD COLUMN deletions INTEGER NOT NULL DEFAULT 0;
ALTER TABLE reviews ADD COLUMN files_changed INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_reviews_author ON reviews(author);
