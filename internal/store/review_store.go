package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ai-code-review/aicr/internal/domain"
)

// CreateReviewInput 创建审查记录的字段。
type CreateReviewInput struct {
	RepoID      int64
	PublicToken string
	CommitSHA   string
	BaseSHA     string
	TargetRef   string
	SourceRef   string
	PRNumber    int
	PRTitle     string
	PRURL       string
	Author      string
	EventType   string
}

// CreateReview 插入一条 pending 审查；同 (repo,commit,target_ref) 已存在则返回 ErrDuplicate。
func (s *Store) CreateReview(ctx context.Context, in CreateReviewInput) (*domain.Review, error) {
	id, err := s.insertID(ctx, `
		INSERT INTO reviews(repo_id, public_token, commit_sha, base_sha, target_ref, source_ref,
			pr_number, pr_title, pr_url, author, event_type, status)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,'pending')`,
		in.RepoID, in.PublicToken, in.CommitSHA, in.BaseSHA, in.TargetRef, in.SourceRef,
		in.PRNumber, in.PRTitle, in.PRURL, in.Author, in.EventType)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return s.GetReview(ctx, id)
}

func (s *Store) GetReview(ctx context.Context, id int64) (*domain.Review, error) {
	row := s.db.QueryRowContext(ctx, s.rebind("SELECT "+reviewColumns()+", r.name FROM reviews rv JOIN repos r ON r.id=rv.repo_id WHERE rv.id=?"), id)
	return scanReview(row.Scan)
}

func (s *Store) GetReviewByPublicToken(ctx context.Context, token string) (*domain.Review, error) {
	row := s.db.QueryRowContext(ctx, s.rebind("SELECT "+reviewColumns()+", r.name FROM reviews rv JOIN repos r ON r.id=rv.repo_id WHERE rv.public_token=?"), token)
	return scanReview(row.Scan)
}

// ListReviews 分页列出审查，可按 repo_id 过滤（0 表示全部）。
func (s *Store) ListReviews(ctx context.Context, repoID int64, limit, offset int) ([]*domain.Review, int, error) {
	var where string
	var args []any
	if repoID > 0 {
		where = "WHERE rv.repo_id=?"
		args = append(args, repoID)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, s.rebind("SELECT COUNT(*) FROM reviews rv "+where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, s.rebind("SELECT "+reviewColumns()+", r.name FROM reviews rv JOIN repos r ON r.id=rv.repo_id "+where+
		" ORDER BY rv.id DESC LIMIT ? OFFSET ?"), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.Review
	for rows.Next() {
		rv, err := scanReview(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, rv)
	}
	return out, total, rows.Err()
}

// ListReviewsByAuthor 列出某作者最近的成功审查（days<=0 表示全部）。
func (s *Store) ListReviewsByAuthor(ctx context.Context, author string, days, limit int) ([]*domain.Review, error) {
	q := "SELECT " + reviewColumns() + ", r.name FROM reviews rv JOIN repos r ON r.id=rv.repo_id " +
		"WHERE rv.author=? AND rv.status='succeeded'"
	args := []any{author}
	if days > 0 {
		q += " AND rv.finished_at >= ?"
		args = append(args, time.Now().Add(-time.Duration(days)*24*time.Hour))
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	q += " ORDER BY rv.id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.rebind(q), args...)
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

func (s *Store) SetReviewStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx,
		s.rebind("UPDATE reviews SET status=?, started_at=CASE WHEN ?='running' THEN "+s.now()+" ELSE started_at END WHERE id=?"),
		status, status, id)
	return err
}

// FinishReviewParams 审查完成后写入结果。
type FinishReviewParams struct {
	Status        string
	Summary       string
	ScoreTotal    int
	ScoreArch     int
	ScoreQuality  int
	ScoreSecurity int
	ScoreMaint    int
	Stats         string
	DiffTruncated bool
	Error         string
	TokensUsed    int
	Additions     int
	Deletions     int
	FilesChanged  int
}

func (s *Store) FinishReview(ctx context.Context, id int64, p FinishReviewParams) error {
	trunc := 0
	if p.DiffTruncated {
		trunc = 1
	}
	_, err := s.db.ExecContext(ctx, s.rebind(`
		UPDATE reviews SET status=?, summary=?, score_total=?, score_arch=?, score_quality=?,
			score_security=?, score_maint=?, stats=?, diff_truncated=?, error=?, tokens_used=?,
			additions=?, deletions=?, files_changed=?, finished_at=`+s.now()+`
		WHERE id=?`),
		p.Status, p.Summary, p.ScoreTotal, p.ScoreArch, p.ScoreQuality,
		p.ScoreSecurity, p.ScoreMaint, p.Stats, trunc, p.Error, p.TokensUsed,
		p.Additions, p.Deletions, p.FilesChanged, id)
	return err
}

func reviewColumns() string {
	return strings.Join([]string{
		"rv.id", "rv.repo_id", "rv.public_token", "rv.commit_sha", "rv.base_sha", "rv.target_ref",
		"rv.source_ref", "rv.pr_number", "rv.pr_title", "rv.pr_url", "rv.author", "rv.event_type",
		"rv.status", "rv.summary", "rv.score_total", "rv.score_arch", "rv.score_quality",
		"rv.score_security", "rv.score_maint", "rv.stats", "rv.diff_truncated", "rv.error",
		"rv.tokens_used", "rv.additions", "rv.deletions", "rv.files_changed",
		"rv.triggered_at", "rv.started_at", "rv.finished_at",
	}, ",")
}

func scanReview(scan func(...any) error) (*domain.Review, error) {
	var rv domain.Review
	var startedAt, finishedAt sql.NullTime
	var trunc int
	err := scan(
		&rv.ID, &rv.RepoID, &rv.PublicToken, &rv.CommitSHA, &rv.BaseSHA, &rv.TargetRef,
		&rv.SourceRef, &rv.PRNumber, &rv.PRTitle, &rv.PRURL, &rv.Author, &rv.EventType,
		&rv.Status, &rv.Summary, &rv.ScoreTotal, &rv.ScoreArch, &rv.ScoreQuality,
		&rv.ScoreSecurity, &rv.ScoreMaint, &rv.Stats, &trunc, &rv.Error,
		&rv.TokensUsed, &rv.Additions, &rv.Deletions, &rv.FilesChanged,
		&rv.TriggeredAt, &startedAt, &finishedAt,
		&rv.RepoName,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan review: %w", err)
	}
	rv.DiffTruncated = trunc == 1
	if startedAt.Valid {
		rv.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		rv.FinishedAt = &finishedAt.Time
	}
	return &rv, nil
}
