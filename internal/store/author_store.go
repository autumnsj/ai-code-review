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
	DisplayName   string    `json:"display_name"`
	Team          string    `json:"team"`
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

// 数据源为 review_author_reports（按 git blame 归属拆分后的个人报告）：
// 多作者推送会为每位参与者贡献一行，评分/问题数/改动量都是该作者自己的。
// 单次单作者审查同样会落一条作者报告，统计口径一致。
const authorSelect = `
	SELECT
		rar.author,
		COALESCE(MAX(a.display_name),'')                  AS display_name,
		COALESCE(MAX(a.team),'')                          AS team,
		COUNT(*)                                          AS review_count,
		COALESCE(AVG(rar.score_total),0)                 AS avg_total,
		COALESCE(AVG(rar.score_arch),0)                  AS avg_arch,
		COALESCE(AVG(rar.score_quality),0)               AS avg_quality,
		COALESCE(AVG(rar.score_security),0)              AS avg_security,
		COALESCE(AVG(rar.score_maint),0)                 AS avg_maint,
		COALESCE(SUM(rar.additions),0)                   AS additions,
		COALESCE(SUM(rar.deletions),0)                   AS deletions,
		COALESCE(SUM(rar.files_changed),0)               AS files_changed,
		0                                                AS tokens_used,
		COALESCE(SUM(rar.findings_count),0)              AS findings_total,
		COALESCE(SUM(rar.critical_count),0)              AS critical,
		COALESCE(SUM(rar.high_count),0)                  AS high,
		COALESCE(SUM(rar.medium_count),0)                AS medium,
		COALESCE(SUM(rar.low_count),0)                   AS low,
		COALESCE(SUM(rar.info_count),0)                  AS info,
		MAX(rv.finished_at)                              AS last_reviewed
	FROM review_author_reports rar
	JOIN reviews rv ON rv.id = rar.review_id
	LEFT JOIN authors a ON a.git_login = rar.author
	WHERE rv.status='succeeded' AND rar.author<>''`

// ListAuthorStats 按作者聚合统计。
func (s *Store) ListAuthorStats(ctx context.Context, f AuthorFilter) ([]*AuthorStats, error) {
	q, args := s.buildAuthorQuery(f)
	orderCol := authorSortColumns[f.Sort]
	if orderCol == "" {
		orderCol = "avg_total"
	}
	q += " GROUP BY rar.author ORDER BY " + orderCol + " DESC, review_count DESC"
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
	q += " GROUP BY rar.author"
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
		b.WriteString(" AND rar.author = ?")
		args = append(args, f.AuthorExact)
	} else if f.Author != "" {
		b.WriteString(" AND rar.author LIKE ?")
		args = append(args, "%"+f.Author+"%")
	}
	return b.String(), args
}

func scanAuthorStats(scan func(...any) error) (*AuthorStats, error) {
	var a AuthorStats
	var last flexibleTime
	err := scan(
		&a.Author, &a.DisplayName, &a.Team, &a.ReviewCount,
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
