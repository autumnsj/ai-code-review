package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strconv"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/server"
	"github.com/ai-code-review/aicr/internal/store"
)

// adminService 装配手动触发、任务管理与 dashboard，供 HTTP 层调用。
type adminService struct {
	store *store.Store
	enq   *reviewEnqueuer
}

func newAdminService(st *store.Store, enq *reviewEnqueuer) *adminService {
	return &adminService{store: st, enq: enq}
}

func newPublicToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// StartReview 手动创建一条审查记录并入队。
func (s *adminService) StartReview(ctx context.Context, repoID int64, in server.StartReviewInput) (int64, string, bool, error) {
	repo, err := s.store.GetRepoByID(ctx, repoID)
	if err != nil {
		return 0, "", false, err
	}
	ref := in.TargetRef
	if ref == "" {
		ref = repo.DefaultBranch
	}
	rv, err := s.store.CreateReview(ctx, store.CreateReviewInput{
		RepoID:      repoID,
		PublicToken: newPublicToken(),
		CommitSHA:   in.CommitSHA,
		BaseSHA:     in.BaseSHA,
		TargetRef:   ref,
		SourceRef:   in.SourceRef,
		Author:      "admin",
		EventType:   "manual",
	})
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return 0, "", true, nil
		}
		return 0, "", false, err
	}
	payload := domain.ReviewPayload{
		ReviewID:  rv.ID,
		RepoID:    repoID,
		CommitSHA: in.CommitSHA,
		BaseSHA:   in.BaseSHA,
		TargetRef: ref,
		SourceRef: in.SourceRef,
		Author:    "admin",
		EventType: "manual",
	}
	key := "review:" + strconv.FormatInt(repoID, 10) + ":" + in.CommitSHA + ":" + ref
	if err := s.enq.EnqueueReview(ctx, payload, key); err != nil {
		return rv.ID, rv.PublicToken, false, err
	}
	return rv.ID, rv.PublicToken, false, nil
}

// ListJobs 透传 store。
func (s *adminService) ListJobs(ctx context.Context, status string, limit, offset int) ([]*domain.Job, int, error) {
	return s.store.ListJobs(ctx, status, limit, offset)
}

// RetryJob 透传 store。
func (s *adminService) RetryJob(ctx context.Context, jobID int64) error {
	return s.store.RetryJob(ctx, jobID)
}

// Dashboard 透传 store 并组装为 server.DashboardSummary。
func (s *adminService) Dashboard(ctx context.Context) (*server.DashboardSummary, error) {
	d, err := s.store.Dashboard(ctx)
	if err != nil {
		return nil, err
	}
	recent, _, err := s.store.ListReviews(ctx, 0, 8, 0)
	if err != nil {
		return nil, err
	}
	type item struct {
		ID         int64   `json:"id"`
		RepoName   string  `json:"repo_name"`
		Status     string  `json:"status"`
		ScoreTotal int     `json:"score_total"`
		CommitSHA  string  `json:"commit_sha"`
		TargetRef  string  `json:"target_ref"`
		FinishedAt *string `json:"finished_at"`
	}
	items := make([]item, 0, len(recent))
	for _, r := range recent {
		var fin *string
		if r.FinishedAt != nil {
			s := r.FinishedAt.Format("2006-01-02 15:04")
			fin = &s
		}
		items = append(items, item{
			ID: r.ID, RepoName: r.RepoName, Status: r.Status, ScoreTotal: r.ScoreTotal,
			CommitSHA: shortSHA(r.CommitSHA), TargetRef: r.TargetRef, FinishedAt: fin,
		})
	}
	return &server.DashboardSummary{
		RepoCount:     d.RepoCount,
		ReviewCount:   d.ReviewCount,
		Succeeded:     d.Succeeded,
		Failed:        d.Failed,
		Running:       d.Pending,
		AvgScore:      roundFloat(d.AvgScore, 1),
		RecentReviews: items,
	}, nil
}

// ListAuthors 返回作者维度统计排行。
func (s *adminService) ListAuthors(ctx context.Context, days int, repoID int64, sortBy string, limit, offset int) ([]server.AuthorSummary, error) {
	rows, err := s.store.ListAuthorStats(ctx, store.AuthorFilter{
		Days: days, RepoID: repoID, Sort: sortBy, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]server.AuthorSummary, 0, len(rows))
	for _, a := range rows {
		out = append(out, toAuthorSummary(a))
	}
	return out, nil
}

// GetAuthor 返回单个作者的统计明细及最近审查。
func (s *adminService) GetAuthor(ctx context.Context, author string, days int, repoID int64) (*server.AuthorDetail, error) {
	a, err := s.store.GetAuthorStats(ctx, store.AuthorFilter{
		Days: days, RepoID: repoID, AuthorExact: author,
	})
	if err != nil {
		return nil, err
	}
	recent, err := s.store.ListReviewsByAuthor(ctx, author, days, 10)
	if err != nil {
		return nil, err
	}
	type recentItem struct {
		ID         int64   `json:"id"`
		RepoName   string  `json:"repo_name"`
		ScoreTotal int     `json:"score_total"`
		Additions  int     `json:"additions"`
		Deletions  int     `json:"deletions"`
		FinishedAt *string `json:"finished_at"`
	}
	items := make([]any, 0, len(recent))
	for _, r := range recent {
		var fin *string
		if r.FinishedAt != nil {
			s := r.FinishedAt.Format("2006-01-02 15:04")
			fin = &s
		}
		items = append(items, recentItem{
			ID: r.ID, RepoName: r.RepoName, ScoreTotal: r.ScoreTotal,
			Additions: r.Additions, Deletions: r.Deletions, FinishedAt: fin,
		})
	}
	summary := toAuthorSummary(a)
	return &server.AuthorDetail{Summary: summary, Recent: items}, nil
}

func toAuthorSummary(a *store.AuthorStats) server.AuthorSummary {
	return server.AuthorSummary{
		Author:        a.Author,
		ReviewCount:   a.ReviewCount,
		AvgTotal:      roundFloat(a.AvgTotal, 1),
		AvgArch:       roundFloat(a.AvgArch, 1),
		AvgQuality:    roundFloat(a.AvgQuality, 1),
		AvgSecurity:   roundFloat(a.AvgSecurity, 1),
		AvgMaint:      roundFloat(a.AvgMaint, 1),
		Additions:     a.Additions,
		Deletions:     a.Deletions,
		FilesChanged:  a.FilesChanged,
		TokensUsed:    a.TokensUsed,
		FindingsTotal: a.FindingsTotal,
		Critical:      a.Critical,
		High:          a.High,
		Medium:        a.Medium,
		Low:           a.Low,
		Info:          a.Info,
		LastReviewed:  a.LastReviewed.Format("2006-01-02 15:04"),
	}
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func roundFloat(v float64, n int) float64 {
	p := 1.0
	for i := 0; i < n; i++ {
		p *= 10
	}
	return float64(int(v*p+0.5)) / p
}
