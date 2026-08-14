package store

import "context"

// DashboardStats 概览统计。
type DashboardStats struct {
	RepoCount   int64
	ReviewCount int64
	Succeeded   int64
	Failed      int64
	Pending     int64
	AvgScore    float64
	TokensUsed  int64
}

// Dashboard 汇总仓库数、审查数、各状态计数与平均分。
func (s *Store) Dashboard(ctx context.Context) (*DashboardStats, error) {
	var d DashboardStats
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM repos),
			(SELECT COUNT(*) FROM reviews),
			(SELECT COUNT(*) FROM reviews WHERE status='succeeded'),
			(SELECT COUNT(*) FROM reviews WHERE status='failed'),
			(SELECT COUNT(*) FROM reviews WHERE status IN ('pending','running')),
			COALESCE((SELECT AVG(score_total) FROM reviews WHERE status='succeeded'), 0),
			COALESCE((SELECT SUM(tokens_used) FROM reviews), 0)
	`).Scan(&d.RepoCount, &d.ReviewCount, &d.Succeeded, &d.Failed, &d.Pending, &d.AvgScore, &d.TokensUsed); err != nil {
		return nil, err
	}
	return &d, nil
}
