package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/ai-code-review/aicr/internal/domain"
)

// ReportRangeTotals 定时日报/周报在一个时间窗口内的计数总览。
type ReportRangeTotals struct {
	ReviewCount int64 // 窗口内结束的审查总数（成功+失败）
	Succeeded   int64
	Failed      int64
	Critical    int64 // 窗口内成功审查发现的 critical 问题数
	High        int64 // 同上，high
}

// ReportRangeStats 聚合 [since, until) 窗口内的审查计数与重点问题计数。
func (s *Store) ReportRangeStats(ctx context.Context, since, until time.Time) (*ReportRangeTotals, error) {
	var t ReportRangeTotals
	err := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN status='succeeded' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END),0)
		FROM reviews
		WHERE finished_at >= ? AND finished_at < ?`), since, until).
		Scan(&t.ReviewCount, &t.Succeeded, &t.Failed)
	if err != nil {
		return nil, err
	}
	err = s.db.QueryRowContext(ctx, s.rebind(`
		SELECT COALESCE(SUM(CASE WHEN f.severity='critical' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN f.severity='high' THEN 1 ELSE 0 END),0)
		FROM findings f
		JOIN reviews rv ON rv.id = f.review_id
		WHERE rv.status='succeeded' AND rv.finished_at >= ? AND rv.finished_at < ?
			AND f.severity IN ('critical','high')`),
		since, until).
		Scan(&t.Critical, &t.High)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListReviewsInRange 列出 [since, until) 窗口内结束的审查（按结束时间倒序），
// 供日报/周报的审查清单使用。
func (s *Store) ListReviewsInRange(ctx context.Context, since, until time.Time, limit int) ([]*domain.Review, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, s.rebind(
		"SELECT "+reviewColumns()+", r.name FROM reviews rv JOIN repos r ON r.id=rv.repo_id "+
			"WHERE rv.finished_at >= ? AND rv.finished_at < ? ORDER BY rv.finished_at DESC LIMIT ?"),
		since, until, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Review
	for rows.Next() {
		rv, err := scanReview(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, rv)
	}
	return out, rows.Err()
}

// RangeFinding 日报/周报重点问题清单中的一行（critical/high）。
type RangeFinding struct {
	RepoName  string
	Severity  string
	FilePath  string
	LineStart int
	Title     string
	Author    string
}

// ListTopFindingsInRange 列出 [since, until) 窗口内成功审查发现的 critical/high 问题，
// critical 优先、同级按审查结束时间倒序，供日报/周报的重点问题清单使用。
func (s *Store) ListTopFindingsInRange(ctx context.Context, since, until time.Time, limit int) ([]*RangeFinding, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT r.name, f.severity, f.file_path, f.line_start, f.title, f.author
		FROM findings f
		JOIN reviews rv ON rv.id = f.review_id
		JOIN repos r ON r.id = rv.repo_id
		WHERE rv.status='succeeded' AND rv.finished_at >= ? AND rv.finished_at < ?
			AND f.severity IN ('critical','high')
		ORDER BY CASE f.severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 ELSE 2 END,
			rv.finished_at DESC
		LIMIT ?`), since, until, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*RangeFinding
	for rows.Next() {
		var f RangeFinding
		var author sql.NullString
		if err := rows.Scan(&f.RepoName, &f.Severity, &f.FilePath, &f.LineStart, &f.Title, &author); err != nil {
			return nil, err
		}
		f.Author = author.String
		out = append(out, &f)
	}
	return out, rows.Err()
}
