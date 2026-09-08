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

// CreateReportInput 落库一条已发送的日报/周报。
type CreateReportInput struct {
	Kind        string    // daily | weekly
	TriggerType string    // scheduled | manual
	PeriodStart time.Time // 统计窗口（北京时间）
	PeriodEnd   time.Time
	Title       string
	Content     string // markdown 正文
	JobID       int64  // 关联 jobs.id，唯一约束防重试重复落库
}

// CreateReport 插入一条报告记录。job_id 唯一：同一 job 失败重试时冲突忽略。
func (s *Store) CreateReport(ctx context.Context, in CreateReportInput) error {
	var query string
	switch s.drv {
	case DriverMySQL:
		query = `INSERT INTO reports(kind, trigger_type, period_start, period_end, title, content, job_id)
			VALUES(?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE id=id`
	default:
		query = `INSERT INTO reports(kind, trigger_type, period_start, period_end, title, content, job_id)
			VALUES(?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(job_id) DO NOTHING`
	}
	_, err := s.db.ExecContext(ctx, s.rebind(query),
		in.Kind, in.TriggerType, in.PeriodStart, in.PeriodEnd, in.Title, in.Content, in.JobID)
	return err
}

// ListReports 分页返回报告记录（按生成时间倒序），不含 content 正文；total 为总数。
func (s *Store) ListReports(ctx context.Context, limit, offset int) ([]*domain.Report, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reports`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, kind, trigger_type, period_start, period_end, title, job_id, created_at
		FROM reports
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?`), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.Report
	for rows.Next() {
		var r domain.Report
		if err := rows.Scan(&r.ID, &r.Kind, &r.TriggerType, &r.PeriodStart, &r.PeriodEnd,
			&r.Title, &r.JobID, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &r)
	}
	return out, total, rows.Err()
}

// GetReport 按 id 查询报告（含 content 正文）；不存在返回 ErrNotFound。
func (s *Store) GetReport(ctx context.Context, id int64) (*domain.Report, error) {
	var r domain.Report
	err := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT id, kind, trigger_type, period_start, period_end, title, content, job_id, created_at
		FROM reports WHERE id = ?`), id).
		Scan(&r.ID, &r.Kind, &r.TriggerType, &r.PeriodStart, &r.PeriodEnd,
			&r.Title, &r.Content, &r.JobID, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}
