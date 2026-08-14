package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ai-code-review/aicr/internal/domain"
)

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type CreateRepoInput struct {
	Provider      domain.Provider
	CloneURL      string
	WebURL        string
	Name          string
	DefaultBranch string
	AccessToken   string
	HookSecret    string
}

func (s *Store) CreateRepo(ctx context.Context, in CreateRepoInput) (*domain.Repo, error) {
	hookToken, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	if in.DefaultBranch == "" {
		in.DefaultBranch = "main"
	}
	if in.Provider == "" {
		in.Provider = domain.ProviderGitHub
	}
	id, err := s.insertID(ctx, `
		INSERT INTO repos(provider, clone_url, web_url, name, default_branch, access_token, hook_token, hook_secret)
		VALUES(?,?,?,?,?,?,?,?)`,
		in.Provider, in.CloneURL, in.WebURL, in.Name, in.DefaultBranch, in.AccessToken, hookToken, in.HookSecret)
	if err != nil {
		return nil, err
	}
	return s.GetRepoByID(ctx, id)
}

func (s *Store) GetRepoByID(ctx context.Context, id int64) (*domain.Repo, error) {
	return s.getRepo(ctx, "id = ?", id)
}

func (s *Store) GetRepoByHookToken(ctx context.Context, token string) (*domain.Repo, error) {
	return s.getRepo(ctx, "hook_token = ?", token)
}

func (s *Store) ListRepos(ctx context.Context) ([]*domain.Repo, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`SELECT `+repoColumns()+` FROM repos ORDER BY id DESC`))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Repo
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateRepo(ctx context.Context, id int64, name, defaultBranch, accessToken, hookSecret *string, status *string) error {
	q := "UPDATE repos SET updated_at=" + s.now()
	args := []any{}
	if name != nil {
		q += ", name=?"
		args = append(args, *name)
	}
	if defaultBranch != nil {
		q += ", default_branch=?"
		args = append(args, *defaultBranch)
	}
	if accessToken != nil {
		q += ", access_token=?"
		args = append(args, *accessToken)
	}
	if hookSecret != nil {
		q += ", hook_secret=?"
		args = append(args, *hookSecret)
	}
	if status != nil {
		q += ", status=?"
		args = append(args, *status)
	}
	q += " WHERE id=?"
	args = append(args, id)
	_, err := s.db.ExecContext(ctx, s.rebind(q), args...)
	return err
}

// ResetHookToken 重新生成 hook token（使旧 hookUrl 失效）。
func (s *Store) ResetHookToken(ctx context.Context, id int64) (string, error) {
	tok, err := randomToken(24)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, s.rebind("UPDATE repos SET hook_token=?, updated_at="+s.now()+" WHERE id=?"), tok, id)
	return tok, err
}

func (s *Store) DeleteRepo(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, s.rebind("DELETE FROM repos WHERE id=?"), id)
	return err
}

func (s *Store) getRepo(ctx context.Context, where string, args ...any) (*domain.Repo, error) {
	row := s.db.QueryRowContext(ctx, s.rebind("SELECT "+repoColumns()+" FROM repos WHERE "+where), args...)
	r, err := scanRepo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func repoColumns() string {
	return strings.Join([]string{
		"id", "provider", "clone_url", "web_url", "name", "default_branch",
		"access_token", "hook_token", "hook_secret", "status",
		"created_at", "updated_at",
	}, ",")
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRepo(sc scanner) (*domain.Repo, error) {
	var r domain.Repo
	err := sc.Scan(
		&r.ID, &r.Provider, &r.CloneURL, &r.WebURL, &r.Name, &r.DefaultBranch,
		&r.AccessToken, &r.HookToken, &r.HookSecret, &r.Status,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan repo: %w", err)
	}
	return &r, nil
}

var (
	ErrNotFound  = errors.New("not found")
	ErrDuplicate = errors.New("duplicate")
)
