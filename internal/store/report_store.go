package store

import (
	"context"
	"time"
)

// ReportRangeTotals 定时日报/周报在一个时间窗口内的汇总指标。
type ReportRangeTotals struct {
	ReviewCount  int64   // 窗口内结束的审查总数（成功+失败）
	Succeeded    int64
	Failed       int64
	AvgScore     float64 // 仅成功审查的综合分均值
	Additions    int64
	Deletions    int64
	FilesChanged int64
	Critical     int64
	High         int64
	Medium       int64
	Low          int64
	Info         int64
}

// ReportRangeStats 聚合 [since, until) 窗口内的审查总览与作者报告累计指标。
// 审查计数取自 reviews（含失败），改动量/问题数取自 review_author_reports
// （仅成功审查有作者报告），与排行榜口径一致。
func (s *Store) ReportRangeStats(ctx context.Context, since, until time.Time) (*ReportRangeTotals, error) {
	var t ReportRangeTotals
	err := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN status='succeeded' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END),0),
			COALESCE(AVG(CASE WHEN status='succeeded' THEN score_total END),0)
		FROM reviews
		WHERE finished_at >= ? AND finished_at < ?`), since, until).
		Scan(&t.ReviewCount, &t.Succeeded, &t.Failed, &t.AvgScore)
	if err != nil {
		return nil, err
	}
	err = s.db.QueryRowContext(ctx, s.rebind(`
		SELECT COALESCE(SUM(rar.additions),0), COALESCE(SUM(rar.deletions),0),
			COALESCE(SUM(rar.files_changed),0),
			COALESCE(SUM(rar.critical_count),0), COALESCE(SUM(rar.high_count),0),
			COALESCE(SUM(rar.medium_count),0), COALESCE(SUM(rar.low_count),0),
			COALESCE(SUM(rar.info_count),0)
		FROM review_author_reports rar
		JOIN reviews rv ON rv.id = rar.review_id
		WHERE rv.status='succeeded' AND rv.finished_at >= ? AND rv.finished_at < ?`),
		since, until).
		Scan(&t.Additions, &t.Deletions, &t.FilesChanged,
			&t.Critical, &t.High, &t.Medium, &t.Low, &t.Info)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
