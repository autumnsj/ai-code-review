package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ai-code-review/aicr/internal/domain"
)

// ListActiveJobReviewIDs 返回所有仍处于活跃状态（pending/running）的 review 类型
// job 所对应的 review_id 集合。reaper 用它排除"其实还在排队/重试/执行中"的审查，
// 避免误杀。从 payload JSON 里解析 review_id（活跃 job 数量很少，在 Go 里解析无需方言相关 SQL）。
func (s *Store) ListActiveJobReviewIDs(ctx context.Context) (map[int64]struct{}, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT payload FROM jobs WHERE status IN ('pending','running') AND kind='review'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]struct{}{}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var p domain.ReviewPayload
		if err := json.Unmarshal([]byte(payload), &p); err == nil && p.ReviewID != 0 {
			out[p.ReviewID] = struct{}{}
		}
	}
	return out, rows.Err()
}

// ListStaleReviewIDs 找出应该被回收的审查 id：
//   - pending 且 triggered_at 早于 pendingBefore（入队后一直没人认领，多为入队失败/worker 未运行）；
//   - running 且 COALESCE(started_at, triggered_at) 早于 runningBefore
//     （超过硬超时窗口仍未结束，进程很可能在写终态前被杀）。
//
// exclude 中的 id（仍有活跃 job）会被跳过，保证不误伤正在重试/执行的审查。
func (s *Store) ListStaleReviewIDs(ctx context.Context, pendingBefore, runningBefore time.Time, exclude map[int64]struct{}) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, status FROM reviews
		WHERE (status='pending' AND triggered_at < ?)
		   OR (status='running' AND COALESCE(started_at, triggered_at) < ?)`),
		pendingBefore, runningBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		var status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		if _, active := exclude[id]; active {
			continue
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
