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

// EnqueueJob 插入待执行任务，幂等键冲突返回 ErrDuplicate。
func (s *Store) EnqueueJob(ctx context.Context, kind, payload, idempotencyKey string, maxAttempts int) (int64, error) {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	id, err := s.insertID(ctx, `
		INSERT INTO jobs(kind, payload, status, max_attempts, idempotency_key, available_at)
		VALUES(?,?, 'pending', ?, ?, `+s.now()+`)`,
		kind, payload, maxAttempts, idempotencyKey)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return 0, ErrDuplicate
		}
		return 0, err
	}
	return id, nil
}

// ClaimJob 原子抢占一个待执行或租约过期的任务，没有可领取的任务时返回 (nil, nil)。
func (s *Store) ClaimJob(ctx context.Context, owner string, lease time.Duration) (*domain.Job, error) {
	until := time.Now().Add(lease)
	// 单条原子 UPDATE...RETURNING；MaxOpenConns=1 保证无竞争。
	row := s.db.QueryRowContext(ctx, s.rebind(`
		UPDATE jobs SET status='running', lease_owner=?, lease_until=?, started_at=`+s.now()+`,
			attempts=attempts+1
		WHERE id = (
			SELECT id FROM jobs
			WHERE (status='pending' AND available_at <= `+s.now()+`)
			   OR (status='running' AND lease_until < `+s.now()+`)
			ORDER BY id LIMIT 1
		)
		RETURNING `+jobColumns()), owner, until)
	j, err := scanJob(row.Scan)
	if errors.Is(err, ErrNotFound) {
		return nil, nil // 没有可领取的任务
	}
	return j, err
}

// Heartbeat 续租。
func (s *Store) Heartbeat(ctx context.Context, jobID int64, owner string, lease time.Duration) error {
	_, err := s.db.ExecContext(ctx,
		s.rebind("UPDATE jobs SET lease_until=? WHERE id=? AND lease_owner=?"),
		time.Now().Add(lease), jobID, owner)
	return err
}

// CompleteJob 标记成功。
func (s *Store) CompleteJob(ctx context.Context, jobID int64) error {
	_, err := s.db.ExecContext(ctx,
		s.rebind("UPDATE jobs SET status='succeeded', finished_at="+s.now()+", lease_owner='', lease_until=NULL WHERE id=?"), jobID)
	return err
}

// FailJob 失败处理：可重试则重置为 pending 并退避，达上限则 dead。
func (s *Store) FailJob(ctx context.Context, jobID int64, jobErr string) error {
	var attempts, maxAttempts int
	err := s.db.QueryRowContext(ctx, s.rebind("SELECT attempts, max_attempts FROM jobs WHERE id=?"), jobID).Scan(&attempts, &maxAttempts)
	if err != nil {
		return err
	}
	if attempts < maxAttempts {
		backoff := time.Duration(1<<min(attempts, 6)) * 10 * time.Second // 10s,20s,40s,80s...
		_, err = s.db.ExecContext(ctx,
			s.rebind("UPDATE jobs SET status='pending', lease_owner='', lease_until=NULL, last_error=?, started_at=NULL, available_at=? WHERE id=?"),
			jobErr, time.Now().Add(backoff), jobID)
		return err
	}
	_, err = s.db.ExecContext(ctx,
		s.rebind("UPDATE jobs SET status='dead', finished_at="+s.now()+", last_error=? WHERE id=?"), jobErr, jobID)
	return err
}

func (s *Store) GetJob(ctx context.Context, id int64) (*domain.Job, error) {
	row := s.db.QueryRowContext(ctx, s.rebind("SELECT "+jobColumns()+" FROM jobs WHERE id=?"), id)
	return scanJob(row.Scan)
}

// ListJobs 分页列出任务，可按 status 过滤（空表示全部）。
func (s *Store) ListJobs(ctx context.Context, status string, limit, offset int) ([]*domain.Job, int, error) {
	var where string
	var args []any
	if status != "" {
		where = "WHERE status=?"
		args = append(args, status)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, s.rebind("SELECT COUNT(*) FROM jobs "+where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, s.rebind("SELECT "+jobColumns()+" FROM jobs "+where+
		" ORDER BY id DESC LIMIT ? OFFSET ?"), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.Job
	for rows.Next() {
		j, err := scanJob(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, j)
	}
	return out, total, rows.Err()
}

// RetryJob 重置失败/dead 的任务为 pending，立即可用并清空 attempts 之外的错误状态。
func (s *Store) RetryJob(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, s.rebind(`
		UPDATE jobs SET status='pending', lease_owner='', lease_until=NULL, last_error='',
			started_at=NULL, available_at=`+s.now()+`
		WHERE id=? AND status IN ('failed','dead','pending')`), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func jobColumns() string {
	return "id, kind, payload, status, lease_owner, lease_until, attempts, max_attempts, " +
		"last_error, idempotency_key, created_at, started_at, finished_at, available_at"
}

func scanJob(scan func(...any) error) (*domain.Job, error) {
	var j domain.Job
	var leaseUntil, started, finished sql.NullTime
	err := scan(
		&j.ID, &j.Kind, &j.Payload, &j.Status, &j.LeaseOwner, &leaseUntil,
		&j.Attempts, &j.MaxAttempts, &j.LastError, &j.IdempotencyKey,
		&j.CreatedAt, &started, &finished, &j.AvailableAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan job: %w", err)
	}
	if leaseUntil.Valid {
		j.LeaseUntil = &leaseUntil.Time
	}
	if started.Valid {
		j.StartedAt = &started.Time
	}
	if finished.Valid {
		j.FinishedAt = &finished.Time
	}
	return &j, nil
}
