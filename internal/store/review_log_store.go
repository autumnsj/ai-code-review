package store

import (
	"context"

	"github.com/ai-code-review/aicr/internal/domain"
)

// maxLogMsgLen 单行日志上限，防止异常输出撑爆数据库。
const maxLogMsgLen = 8192

// AppendReviewLog 追加一行审查执行日志。message 超长时截断。
func (s *Store) AppendReviewLog(ctx context.Context, reviewID int64, level, msg string) error {
	if len(msg) > maxLogMsgLen {
		msg = msg[:maxLogMsgLen] + "...[truncated]"
	}
	_, err := s.db.ExecContext(ctx, s.rebind(
		`INSERT INTO review_logs(review_id, level, message) VALUES(?,?,?)`),
		reviewID, normalizeLevel(level), msg)
	return err
}

// ListReviewLogs 返回某审查自 sinceID（不含）之后的日志，按 id 升序，最多 limit 条。
func (s *Store) ListReviewLogs(ctx context.Context, reviewID, sinceID int64, limit int) ([]domain.ReviewLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, s.rebind(
		`SELECT id, review_id, level, message, created_at FROM review_logs
		 WHERE review_id=? AND id>? ORDER BY id ASC LIMIT ?`),
		reviewID, sinceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// 初始化为非 nil 空切片，确保无日志时 JSON 序列化为 [] 而非 null，避免前端对 null 取 .length 崩溃。
	out := make([]domain.ReviewLog, 0)
	for rows.Next() {
		var l domain.ReviewLog
		if err := rows.Scan(&l.ID, &l.ReviewID, &l.Level, &l.Message, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func normalizeLevel(level string) string {
	switch level {
	case "warn", "error", "info":
		return level
	default:
		return "info"
	}
}
