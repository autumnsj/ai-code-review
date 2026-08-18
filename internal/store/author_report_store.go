package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ai-code-review/aicr/internal/domain"
)

// UpsertAuthorReportParams 写入/更新一条按作者拆分的审查报告所需字段。
type UpsertAuthorReportParams struct {
	ReviewID        int64
	Author          string // email 稳定键
	AuthorName      string
	PublicToken     string
	Summary         string
	ScoreTotal      int
	ScoreArch       int
	ScoreQuality    int
	ScoreSecurity   int
	ScoreMaint      int
	ScoreDimensions string
	FindingsCount   int
	CriticalCount   int
	HighCount       int
	MediumCount     int
	LowCount        int
	InfoCount       int
	Additions       int
	Deletions       int
	FilesChanged    int
}

const authorReportColumns = `rar.id, rar.review_id, rar.author, rar.author_name, rar.public_token,
	r.name AS repo_name, rv.commit_sha, rv.base_sha, rv.target_ref,
	rar.summary, rar.score_total, rar.score_arch, rar.score_quality, rar.score_security, rar.score_maint,
	rar.score_dimensions, rar.findings_count, rar.critical_count, rar.high_count, rar.medium_count,
	rar.low_count, rar.info_count, rar.additions, rar.deletions, rar.files_changed,
	rv.triggered_at, rar.created_at`

// UpsertAuthorReport 按 (review_id, author) upsert 一条作者报告；存在则更新。
func (s *Store) UpsertAuthorReport(ctx context.Context, p UpsertAuthorReportParams) error {
	switch s.drv {
	case DriverPostgres:
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO review_author_reports(
				review_id, author, author_name, public_token, summary,
				score_total, score_arch, score_quality, score_security, score_maint, score_dimensions,
				findings_count, critical_count, high_count, medium_count, low_count, info_count,
				additions, deletions, files_changed)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
			ON CONFLICT(review_id, author) DO UPDATE SET
				author_name=EXCLUDED.author_name, public_token=EXCLUDED.public_token,
				summary=EXCLUDED.summary, score_total=EXCLUDED.score_total,
				score_arch=EXCLUDED.score_arch, score_quality=EXCLUDED.score_quality,
				score_security=EXCLUDED.score_security, score_maint=EXCLUDED.score_maint,
				score_dimensions=EXCLUDED.score_dimensions,
				findings_count=EXCLUDED.findings_count, critical_count=EXCLUDED.critical_count,
				high_count=EXCLUDED.high_count, medium_count=EXCLUDED.medium_count,
				low_count=EXCLUDED.low_count, info_count=EXCLUDED.info_count,
				additions=EXCLUDED.additions, deletions=EXCLUDED.deletions, files_changed=EXCLUDED.files_changed`,
			p.ReviewID, p.Author, p.AuthorName, p.PublicToken, p.Summary,
			p.ScoreTotal, p.ScoreArch, p.ScoreQuality, p.ScoreSecurity, p.ScoreMaint, p.ScoreDimensions,
			p.FindingsCount, p.CriticalCount, p.HighCount, p.MediumCount, p.LowCount, p.InfoCount,
			p.Additions, p.Deletions, p.FilesChanged)
		return err
	case DriverMySQL:
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO review_author_reports(
				review_id, author, author_name, public_token, summary,
				score_total, score_arch, score_quality, score_security, score_maint, score_dimensions,
				findings_count, critical_count, high_count, medium_count, low_count, info_count,
				additions, deletions, files_changed)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
				author_name=VALUES(author_name), public_token=VALUES(public_token),
				summary=VALUES(summary), score_total=VALUES(score_total),
				score_arch=VALUES(score_arch), score_quality=VALUES(score_quality),
				score_security=VALUES(score_security), score_maint=VALUES(score_maint),
				score_dimensions=VALUES(score_dimensions),
				findings_count=VALUES(findings_count), critical_count=VALUES(critical_count),
				high_count=VALUES(high_count), medium_count=VALUES(medium_count),
				low_count=VALUES(low_count), info_count=VALUES(info_count),
				additions=VALUES(additions), deletions=VALUES(deletions), files_changed=VALUES(files_changed)`,
			p.ReviewID, p.Author, p.AuthorName, p.PublicToken, p.Summary,
			p.ScoreTotal, p.ScoreArch, p.ScoreQuality, p.ScoreSecurity, p.ScoreMaint, p.ScoreDimensions,
			p.FindingsCount, p.CriticalCount, p.HighCount, p.MediumCount, p.LowCount, p.InfoCount,
			p.Additions, p.Deletions, p.FilesChanged)
		return err
	default:
		// SQLite 无 ON CONFLICT(review_id, author) 的命名约束，用 DELETE+INSERT 简化（单事务）。
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `DELETE FROM review_author_reports WHERE review_id=? AND author=?`,
			p.ReviewID, p.Author); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO review_author_reports(
				review_id, author, author_name, public_token, summary,
				score_total, score_arch, score_quality, score_security, score_maint, score_dimensions,
				findings_count, critical_count, high_count, medium_count, low_count, info_count,
				additions, deletions, files_changed)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.ReviewID, p.Author, p.AuthorName, p.PublicToken, p.Summary,
			p.ScoreTotal, p.ScoreArch, p.ScoreQuality, p.ScoreSecurity, p.ScoreMaint, p.ScoreDimensions,
			p.FindingsCount, p.CriticalCount, p.HighCount, p.MediumCount, p.LowCount, p.InfoCount,
			p.Additions, p.Deletions, p.FilesChanged); err != nil {
			return err
		}
		return tx.Commit()
	}
}

// DeleteAuthorReports 删除某次审查的所有作者报告（重新审查时清空旧结果）。
func (s *Store) DeleteAuthorReports(ctx context.Context, reviewID int64) error {
	_, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM review_author_reports WHERE review_id=?`), reviewID)
	return err
}

// ListAuthorReportsByReview 返回某次审查的全部作者报告，按问题数降序。
func (s *Store) ListAuthorReportsByReview(ctx context.Context, reviewID int64) ([]*domain.ReviewAuthorReport, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT `+authorReportColumns+`
		FROM review_author_reports rar
		JOIN reviews rv ON rv.id = rar.review_id
		JOIN repos r ON r.id = rv.repo_id
		WHERE rar.review_id=?
		ORDER BY rar.findings_count DESC, rar.score_total ASC`), reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuthorReports(rows)
}

// GetAuthorReportByToken 按公开 token 取单个作者报告（用于公开报告页）。
func (s *Store) GetAuthorReportByToken(ctx context.Context, token string) (*domain.ReviewAuthorReport, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT `+authorReportColumns+`
		FROM review_author_reports rar
		JOIN reviews rv ON rv.id = rar.review_id
		JOIN repos r ON r.id = rv.repo_id
		WHERE rar.public_token=?`), token)
	ar, err := scanAuthorReport(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return ar, nil
}

func scanAuthorReports(rows *sql.Rows) ([]*domain.ReviewAuthorReport, error) {
	out := make([]*domain.ReviewAuthorReport, 0)
	for rows.Next() {
		ar, err := scanAuthorReport(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, ar)
	}
	return out, rows.Err()
}

func scanAuthorReport(scan func(...any) error) (*domain.ReviewAuthorReport, error) {
	var ar domain.ReviewAuthorReport
	var dimJSON string
	var finished sql.NullTime // created_at 用作"完成时间"展示
	err := scan(
		&ar.ID, &ar.ReviewID, &ar.Author, &ar.AuthorName, &ar.PublicToken,
		&ar.RepoName, &ar.CommitSHA, &ar.BaseSHA, &ar.TargetRef,
		&ar.Summary, &ar.ScoreTotal, &ar.ScoreArch, &ar.ScoreQuality, &ar.ScoreSecurity, &ar.ScoreMaint,
		&dimJSON, &ar.FindingsCount, &ar.CriticalCount, &ar.HighCount, &ar.MediumCount,
		&ar.LowCount, &ar.InfoCount, &ar.Additions, &ar.Deletions, &ar.FilesChanged,
		&ar.TriggeredAt, &finished,
	)
	if err != nil {
		return nil, fmt.Errorf("scan author report: %w", err)
	}
	if dimJSON != "" {
		_ = json.Unmarshal([]byte(dimJSON), &ar.ScoreDimensions)
	}
	if finished.Valid {
		ar.FinishedAt = &finished.Time
	}
	return &ar, nil
}
