package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config 持有启动期配置（全部来自环境变量）。
// 运行期可在 UI 修改的业务配置存于 DB settings 表。
type Config struct {
	Port              string
	DataDir           string
	AdminPassword     string
	BaseURL           string
	JWTSecret         string
	DBDriver          string // 无头引导：sqlite | postgres | mysql（留空则首次启动走 Web 向导）
	DBDSN             string // 无头引导：sqlite 文件路径 / postgres DSN / mysql DSN（留空则 sqlite 用默认路径）
	WorkerConcurrency int
	LogLevel          string
	PiAgentBin        string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:              env("AICR_PORT", "8080"),
		DataDir:           env("AICR_DATA_DIR", "./data"),
		AdminPassword:     env("AICR_ADMIN_PASSWORD", "admin123"),
		BaseURL:           env("AICR_BASE_URL", "http://localhost:8080"),
		JWTSecret:         os.Getenv("AICR_JWT_SECRET"),
		DBDriver:          os.Getenv("AICR_DB_DRIVER"),
		DBDSN:             os.Getenv("AICR_DB_DSN"),
		WorkerConcurrency: envInt("AICR_WORKER_CONCURRENCY", 2),
		LogLevel:          env("AICR_LOG_LEVEL", "info"),
		PiAgentBin:        env("AICR_PI_AGENT_BIN", "pi-agent"),
	}
	if cfg.WorkerConcurrency < 1 {
		cfg.WorkerConcurrency = 1
	}
	if cfg.AdminPassword == "" {
		return nil, fmt.Errorf("AICR_ADMIN_PASSWORD 不能为空")
	}
	if cfg.DBDriver != "" && cfg.DBDriver != "sqlite" && cfg.DBDriver != "postgres" && cfg.DBDriver != "mysql" {
		return nil, fmt.Errorf("AICR_DB_DRIVER 仅支持 sqlite、postgres 或 mysql，当前: %s", cfg.DBDriver)
	}
	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
