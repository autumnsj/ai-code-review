package store

import (
	"context"

	"github.com/ai-code-review/aicr/internal/domain"
)

// InsertFindings 批量插入一次审查的所有 finding（单事务）。
func (s *Store) InsertFindings(ctx context.Context, reviewID int64, findings []*domain.Finding) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, s.rebind(`
		INSERT INTO findings(review_id, source, rule_id, severity, category, file_path,
			line_start, line_end, title, message, snippet, suggestion, confidence)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, f := range findings {
		_, err := stmt.ExecContext(ctx, reviewID, f.Source, f.RuleID, f.Severity, f.Category,
			f.FilePath, f.LineStart, f.LineEnd, f.Title, f.Message, f.Snippet, f.Suggestion, f.Confidence)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListFindings(ctx context.Context, reviewID int64) ([]*domain.Finding, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, review_id, source, rule_id, severity, category, file_path,
			line_start, line_end, title, message, snippet, suggestion, confidence, created_at
		FROM findings WHERE review_id=? ORDER BY
			CASE severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END,
			file_path, line_start`), reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Finding
	for rows.Next() {
		var f domain.Finding
		if err := rows.Scan(&f.ID, &f.ReviewID, &f.Source, &f.RuleID, &f.Severity, &f.Category,
			&f.FilePath, &f.LineStart, &f.LineEnd, &f.Title, &f.Message, &f.Snippet, &f.Suggestion,
			&f.Confidence, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}
