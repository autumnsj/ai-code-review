// Package bootstrap 管理首次初始化引导文件（$DATA_DIR/aicr.json）。
// 它记录数据库方言与连接串，是启动时「用哪个库」的唯一来源——不能存在数据库里。
package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config 持久化的引导配置。
type Config struct {
	Driver string `json:"driver"` // sqlite | postgres
	DSN    string `json:"dsn"`    // sqlite: 文件路径；postgres: 连接串
}

// Path 返回引导文件的完整路径。
func Path(dataDir string) string {
	return filepath.Join(dataDir, "aicr.json")
}

// Exists 判断引导文件是否存在。
func Exists(dataDir string) bool {
	_, err := os.Stat(Path(dataDir))
	return err == nil
}

// Load 读取引导配置。
func Load(dataDir string) (*Config, error) {
	b, err := os.ReadFile(Path(dataDir))
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse bootstrap config: %w", err)
	}
	if cfg.Driver == "" {
		return nil, errors.New("bootstrap config missing driver")
	}
	return &cfg, nil
}

// Save 原子写入引导配置（0600 权限）。
func Save(dataDir string, cfg Config) error {
	if cfg.Driver == "" {
		return errors.New("driver is required")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dataDir, ".aicr-bootstrap-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, Path(dataDir))
}
