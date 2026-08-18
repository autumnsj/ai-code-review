package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ai-code-review/aicr/internal/domain"
)

type CreateCredentialInput struct {
	Name        string
	Type        string // ssh | https_token
	Secret      string // SSH 私钥 PEM / HTTPS token
	PublicKey   string
	Fingerprint string
	Provider    string // https_token 时：平台 github|gitlab|gitee|gitea
	APIBaseURL  string // 自建实例 API 地址
}

// CreateCredential 新增凭据。
func (s *Store) CreateCredential(ctx context.Context, in CreateCredentialInput) (*domain.Credential, error) {
	id, err := s.insertID(ctx, `
		INSERT INTO credentials(name, type, secret, public_key, fingerprint, provider, api_base_url)
		VALUES(?,?,?,?,?,?,?)`,
		in.Name, in.Type, in.Secret, in.PublicKey, in.Fingerprint, in.Provider, in.APIBaseURL)
	if err != nil {
		return nil, err
	}
	return s.GetCredential(ctx, id)
}

// credentialListColumns 不含 secret（列表不回传明文）。
func credentialListColumns() string {
	return strings.Join([]string{
		"id", "name", "type", "public_key", "fingerprint", "provider", "api_base_url", "created_at", "updated_at",
	}, ",")
}

func credentialAllColumns() string {
	return strings.Join([]string{
		"id", "name", "type", "secret", "public_key", "fingerprint", "provider", "api_base_url", "created_at", "updated_at",
	}, ",")
}

// ListCredentials 返回所有凭据（不含 secret 明文）。
func (s *Store) ListCredentials(ctx context.Context) ([]*domain.Credential, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(
		`SELECT `+credentialListColumns()+` FROM credentials ORDER BY id DESC`))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Credential
	for rows.Next() {
		c, err := scanCredential(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCredential 返回单个凭据（含 secret，供 worker 使用）。
func (s *Store) GetCredential(ctx context.Context, id int64) (*domain.Credential, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(
		`SELECT `+credentialAllColumns()+` FROM credentials WHERE id = ?`), id)
	c, err := scanCredential(row, true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// UpdateCredential 更新名称/密钥。name、secret 为 nil 表示不修改；secret 传空字符串视为不修改
// （前端「留空不修改」会提交空串；如需清空，由显式接口处理，此处不开放）。
func (s *Store) UpdateCredential(ctx context.Context, id int64, name, secret, provider, apiBaseURL *string) error {
	q := "UPDATE credentials SET updated_at=" + s.now()
	args := []any{}
	if name != nil {
		q += ", name=?"
		args = append(args, *name)
	}
	if secret != nil && *secret != "" {
		q += ", secret=?"
		args = append(args, *secret)
	}
	if provider != nil {
		q += ", provider=?"
		args = append(args, *provider)
	}
	if apiBaseURL != nil {
		q += ", api_base_url=?"
		args = append(args, *apiBaseURL)
	}
	q += " WHERE id=?"
	args = append(args, id)
	_, err := s.db.ExecContext(ctx, s.rebind(q), args...)
	return err
}

// UpdateCredentialKeyMaterial 替换 SSH 凭据的私钥/公钥/指纹（粘贴新私钥时使用）。
func (s *Store) UpdateCredentialKeyMaterial(ctx context.Context, id int64, secret, publicKey, fingerprint string) error {
	_, err := s.db.ExecContext(ctx, s.rebind(
		`UPDATE credentials SET secret=?, public_key=?, fingerprint=?, updated_at=`+s.now()+` WHERE id=?`),
		secret, publicKey, fingerprint, id)
	return err
}

// DeleteCredential 删除凭据。被仓库引用时由外键 ON DELETE SET NULL 自动解绑。
func (s *Store) DeleteCredential(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM credentials WHERE id = ?`), id)
	return err
}

type credScanner interface {
	Scan(dest ...any) error
}

func scanCredential(sc credScanner, withSecret bool) (*domain.Credential, error) {
	var c domain.Credential
	dests := []any{&c.ID, &c.Name, &c.Type}
	if withSecret {
		dests = append(dests, &c.Secret)
	}
	dests = append(dests, &c.PublicKey, &c.Fingerprint, &c.Provider, &c.APIBaseURL, &c.CreatedAt, &c.UpdatedAt)
	if err := sc.Scan(dests...); err != nil {
		return nil, fmt.Errorf("scan credential: %w", err)
	}
	return &c, nil
}
