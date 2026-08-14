package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// flexibleTime 容忍不同驱动对聚合时间列的返回类型：
// PostgreSQL 对 MAX(timestamptz) 返回 time.Time；modernc sqlite 对 MAX(...) 表达式
// 因无声明列类型而返回字符串。两种都解析为 time.Time。
type flexibleTime struct {
	t time.Time
}

func (f *flexibleTime) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		f.t = time.Time{}
		return nil
	case time.Time:
		f.t = x
		return nil
	case string:
		if x == "" {
			f.t = time.Time{}
			return nil
		}
		for _, layout := range []string{
			"2006-01-02 15:04:05.999999999-07:04:05",
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02T15:04:05.999999999-07:00",
			"2006-01-02 15:04:05",
			time.RFC3339Nano,
			time.RFC3339,
		} {
			if t, err := time.Parse(layout, x); err == nil {
				f.t = t
				return nil
			}
		}
		return fmt.Errorf("cannot parse time %q", x)
	default:
		return fmt.Errorf("unsupported time type %T", v)
	}
}

// AuthorStats 单个作者的汇总指标（仅统计 succeeded 的审查）。
type AuthorStats struct {
	Author        string    `json:"author"`
	ReviewCount   int64     `json:"review_count"`
	AvgTotal      float64   `json:"avg_total"`
	AvgArch       float64   `json:"avg_arch"`
	AvgQuality    float64   `json:"avg_quality"`
	AvgSecurity   float64   `json:"avg_security"`
	AvgMaint      float64   `json:"avg_maint"`
	Additions     int64     `json:"additions"`
	Deletions     int64     `json:"deletions"`
	FilesChanged  int64     `json:"files_changed"`
	TokensUsed    int64     `json:"tokens_used"`
	FindingsTotal int64     `json:"findings_total"`
	Critical      int64     `json:"critical"`
	High          int64     `json:"high"`
	Medium        int64     `json:"medium"`
	Low           int64     `json:"low"`
	Info          int64     `json:"info"`
	LastReviewed  time.Time `json:"last_reviewed"`
}

// AuthorFilter 作者统计的过滤条件。
type AuthorFilter struct {
	Days        int    // 0 表示全部
	RepoID      int64  // 0 表示全部
	Author      string // 模糊匹配；AuthorExact 非空时精确匹配
	AuthorExact string
	Sort        string // avg_score|additions|deletions|review_count|findings
	Limit       int
	Offset      int
}

var authorSortColumns = map[string]string{
	"avg_score":    "avg_total",
	"additions":    "additions",
	"deletions":    "deletions",
	"review_count": "review_count",
	"findings":     "findings_total",
}

// findingsAgg 预先按 review 聚合问题数，避免 LEFT JOIN 把审查级的增删行按问题数重复累加。
const findingsAgg = `
	LEFT JOIN (
		SELECT review_id,
			COUNT(*) AS f_total,
			SUM(CASE WHEN severity='critical' THEN 1 ELSE 0 END) AS f_critical,
			SUM(CASE WHEN severity='high'     THEN 1 ELSE 0 END) AS f_high,
			SUM(CASE WHEN severity='medium'   THEN 1 ELSE 0 END) AS f_medium,
			SUM(CASE WHEN severity='low'      THEN 1 ELSE 0 END) AS f_low,
			SUM(CASE WHEN severity='info'     THEN 1 ELSE 0 END) AS f_info
		FROM findings GROUP BY review_id
	) fa ON fa.review_id = rv.id`

const authorSelect = `
	SELECT
		rv.author,
		COUNT(*)                                          AS review_count,
		COALESCE(AVG(rv.score_total),0)                  AS avg_total,
		COALESCE(AVG(rv.score_arch),0)                   AS avg_arch,
		COALESCE(AVG(rv.score_quality),0)                AS avg_quality,
		COALESCE(AVG(rv.score_security),0)               AS avg_security,
		COALESCE(AVG(rv.score_maint),0)                  AS avg_maint,
		COALESCE(SUM(rv.additions),0)                    AS additions,
		COALESCE(SUM(rv.deletions),0)                    AS deletions,
		COALESCE(SUM(rv.files_changed),0)                AS files_changed,
		COALESCE(SUM(rv.tokens_used),0)                  AS tokens_used,
		COALESCE(SUM(fa.f_total),0)                      AS findings_total,
		COALESCE(SUM(fa.f_critical),0)                   AS critical,
		COALESCE(SUM(fa.f_high),0)                       AS high,
		COALESCE(SUM(fa.f_medium),0)                     AS medium,
		COALESCE(SUM(fa.f_low),0)                        AS low,
		COALESCE(SUM(fa.f_info),0)                       AS info,
		MAX(rv.finished_at)                              AS last_reviewed
	FROM reviews rv` + findingsAgg + `
	WHERE rv.status='succeeded' AND rv.author<>''`

// ListAuthorStats 按作者聚合统计。
func (s *Store) ListAuthorStats(ctx context.Context, f AuthorFilter) ([]*AuthorStats, error) {
	q, args := s.buildAuthorQuery(f)
	orderCol := authorSortColumns[f.Sort]
	if orderCol == "" {
		orderCol = "avg_total"
	}
	q += " GROUP BY rv.author ORDER BY " + orderCol + " DESC, review_count DESC"
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q += " LIMIT ? OFFSET ?"
	args = append(args, limit, f.Offset)

	rows, err := s.db.QueryContext(ctx, s.rebind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AuthorStats
	for rows.Next() {
		a, err := scanAuthorStats(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAuthorStats 返回单个作者的聚合。
func (s *Store) GetAuthorStats(ctx context.Context, f AuthorFilter) (*AuthorStats, error) {
	q, args := s.buildAuthorQuery(f)
	q += " GROUP BY rv.author"
	row := s.db.QueryRowContext(ctx, s.rebind(q), args...)
	return scanAuthorStats(row.Scan)
}

// buildAuthorQuery 构造 WHERE 子句与参数。
func (s *Store) buildAuthorQuery(f AuthorFilter) (string, []any) {
	var b strings.Builder
	b.WriteString(authorSelect)
	args := []any{}
	if f.Days > 0 {
		b.WriteString(" AND rv.finished_at >= ?")
		args = append(args, time.Now().Add(-time.Duration(f.Days)*24*time.Hour))
	}
	if f.RepoID > 0 {
		b.WriteString(" AND rv.repo_id = ?")
		args = append(args, f.RepoID)
	}
	if f.AuthorExact != "" {
		b.WriteString(" AND rv.author = ?")
		args = append(args, f.AuthorExact)
	} else if f.Author != "" {
		b.WriteString(" AND rv.author LIKE ?")
		args = append(args, "%"+f.Author+"%")
	}
	return b.String(), args
}

func scanAuthorStats(scan func(...any) error) (*AuthorStats, error) {
	var a AuthorStats
	var last flexibleTime
	err := scan(
		&a.Author, &a.ReviewCount,
		&a.AvgTotal, &a.AvgArch, &a.AvgQuality, &a.AvgSecurity, &a.AvgMaint,
		&a.Additions, &a.Deletions, &a.FilesChanged, &a.TokensUsed,
		&a.FindingsTotal, &a.Critical, &a.High, &a.Medium, &a.Low, &a.Info,
		&last,
	)
	if err != nil {
		return nil, err
	}
	a.LastReviewed = last.t
	return &a, nil
}

var _ sql.Scanner = (*flexibleTime)(nil)
