package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ai-code-review/aicr/internal/domain"
)

type CreateAuthorInput struct {
	GitLogin    string
	DisplayName string
	Team        string
	Note        string
	Active      bool
}

type UpdateAuthorInput struct {
	DisplayName *string
	Team        *string
	Note        *string
	Active      *bool
}

var authorColumns = strings.Join([]string{
	"id", "git_login", "display_name", "team", "note", "active", "created_at", "updated_at",
}, ",")

func scanAuthor(sc credScanner) (*domain.Author, error) {
	var a domain.Author
	var active int
	err := sc.Scan(&a.ID, &a.GitLogin, &a.DisplayName, &a.Team, &a.Note, &active, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.Active = active == 1
	return &a, nil
}

// ListAuthors 列出所有成员备注。
func (s *Store) ListAuthors(ctx context.Context) ([]*domain.Author, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+authorColumns+` FROM authors ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Author
	for rows.Next() {
		a, err := scanAuthor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAuthorByLogin 按 git_login 查询；不存在返回 ErrNotFound。
func (s *Store) GetAuthorByLogin(ctx context.Context, login string) (*domain.Author, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`SELECT `+authorColumns+` FROM authors WHERE git_login=?`), login)
	a, err := scanAuthor(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// CreateAuthor 新增成员备注。
func (s *Store) CreateAuthor(ctx context.Context, in CreateAuthorInput) (*domain.Author, error) {
	active := 1
	if !in.Active {
		active = 0
	}
	id, err := s.insertID(ctx, s.rebind(
		`INSERT INTO authors(git_login, display_name, team, note, active) VALUES(?,?,?,?,?)`),
		in.GitLogin, in.DisplayName, in.Team, in.Note, active)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return s.GetAuthorByID(ctx, id)
}

// UpdateAuthor 部分更新成员备注。
func (s *Store) UpdateAuthor(ctx context.Context, id int64, in UpdateAuthorInput) error {
	q := "UPDATE authors SET updated_at=" + s.now()
	args := []any{}
	if in.DisplayName != nil {
		q += ", display_name=?"
		args = append(args, *in.DisplayName)
	}
	if in.Team != nil {
		q += ", team=?"
		args = append(args, *in.Team)
	}
	if in.Note != nil {
		q += ", note=?"
		args = append(args, *in.Note)
	}
	if in.Active != nil {
		q += ", active=?"
		args = append(args, boolToInt(*in.Active))
	}
	q += " WHERE id=?"
	args = append(args, id)
	res, err := s.db.ExecContext(ctx, s.rebind(q), args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAuthor 删除成员备注。
func (s *Store) DeleteAuthor(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM authors WHERE id=?`), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListUnknownLogins 返回审查记录中出现、但未在 authors 表中备注的 git_login。
func (s *Store) ListUnknownLogins(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT rv.author FROM reviews rv
		LEFT JOIN authors a ON a.git_login = rv.author
		WHERE rv.author<>'' AND a.git_login IS NULL
		ORDER BY rv.author`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetAuthorByID 按主键查询成员备注；不存在返回 ErrNotFound。
func (s *Store) GetAuthorByID(ctx context.Context, id int64) (*domain.Author, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`SELECT `+authorColumns+` FROM authors WHERE id=?`), id)
	a, err := scanAuthor(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan author: %w", err)
	}
	return a, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
