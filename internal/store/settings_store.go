package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func (s *Store) GetSetting(ctx context.Context, key string, dst any) error {
	var raw string
	err := s.db.QueryRowContext(ctx, s.rebind("SELECT value FROM settings WHERE key=?"), key).Scan(&raw)
	if err != nil {
		return err
	}
	if dst == nil {
		return nil
	}
	return json.Unmarshal([]byte(raw), dst)
}

func (s *Store) SetSetting(ctx context.Context, key string, val any) error {
	raw, err := json.Marshal(val)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO settings(key, value, updated_at) VALUES(?, ?, `+s.now()+`)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=`+s.now()),
		key, string(raw))
	return err
}

// IsSettingNotFound 判断是否因 key 不存在而报错。
func IsSettingNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
