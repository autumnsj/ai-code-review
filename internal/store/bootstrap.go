package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/ai-code-review/aicr/internal/auth"
	"github.com/ai-code-review/aicr/internal/domain"
)

// EnsureServerSettings 初始化服务级设置：管理员密码 hash、JWT secret、baseURL。
// 已存在则不覆盖（密码修改走 UI）。jwtSecretEnv 非空时用它，否则随机生成。
func (s *Store) EnsureServerSettings(ctx context.Context, adminPassword, baseURL, jwtSecretEnv string) (*domain.ServerConfig, error) {
	var cfg domain.ServerConfig
	err := s.GetSetting(ctx, "server", &cfg)
	if err != nil && !IsSettingNotFound(err) {
		return nil, err
	}

	changed := false
	if cfg.AdminPasswordHash == "" {
		h, err := auth.HashPassword(adminPassword)
		if err != nil {
			return nil, err
		}
		cfg.AdminPasswordHash = h
		changed = true
	}
	if cfg.JWTSecret == "" {
		if jwtSecretEnv != "" {
			cfg.JWTSecret = jwtSecretEnv
		} else {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				return nil, err
			}
			cfg.JWTSecret = hex.EncodeToString(b)
		}
		changed = true
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = baseURL
		changed = true
	}
	if changed {
		if err := s.SetSetting(ctx, "server", cfg); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

// UpdateBaseURL 更新对外基础地址。
func (s *Store) UpdateBaseURL(ctx context.Context, baseURL string) error {
	cfg, err := s.GetServerSettings(ctx)
	if err != nil {
		return err
	}
	cfg.BaseURL = baseURL
	return s.SetSetting(ctx, "server", cfg)
}

func (s *Store) GetServerSettings(ctx context.Context) (*domain.ServerConfig, error) {
	var cfg domain.ServerConfig
	if err := s.GetSetting(ctx, "server", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
