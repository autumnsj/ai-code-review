package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

var (
	//go:embed migrations/sqlite/*.sql
	sqliteMigrations embed.FS
	//go:embed migrations/postgres/*.sql
	postgresMigrations embed.FS
	//go:embed migrations/mysql/*.sql
	mysqlMigrations embed.FS
)

// Logger 是迁移过程使用的最小日志接口。*zap.SugaredLogger 天然满足。
type Logger interface {
	Infof(format string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Infof(string, ...any) {}

func migrationFSFor(driver Driver) (fs.FS, string) {
	switch driver {
	case DriverPostgres:
		return postgresMigrations, "migrations/postgres"
	case DriverMySQL:
		return mysqlMigrations, "migrations/mysql"
	default:
		return sqliteMigrations, "migrations/sqlite"
	}
}

// migrationFiles 返回按版本号排序的 (version, filename) 列表。
func migrationFiles(driver Driver) ([]string, error) {
	root, sub := migrationFSFor(driver)
	entries, err := fs.ReadDir(root, sub)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// LatestMigrationVersion 返回当前二进制内置的最高迁移版本号。
func LatestMigrationVersion(driver Driver) (int, error) {
	names, err := migrationFiles(driver)
	if err != nil {
		return 0, err
	}
	latest := 0
	for _, name := range names {
		v, err := parseVersion(name)
		if err != nil {
			return 0, err
		}
		if v > latest {
			latest = v
		}
	}
	return latest, nil
}

func parseVersion(name string) (int, error) {
	idx := strings.IndexByte(name, '_')
	if idx < 0 {
		return 0, fmt.Errorf("parse migration version %q: missing '_' separator", name)
	}
	v, err := strconv.Atoi(name[:idx])
	if err != nil {
		return 0, fmt.Errorf("parse migration version %q: %w", name, err)
	}
	return v, nil
}

// currentDBVersion 读取已应用的最高迁移版本；无记录时返回 0。
func currentDBVersion(db *sql.DB) (int, error) {
	var max sql.NullInt64
	if err := db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&max); err != nil {
		return 0, err
	}
	return int(max.Int64), nil
}

// migrate 执行所有未应用的前向迁移，返回迁移前后的版本号。
// 若数据库版本高于二进制内置版本，返回 ErrDBTooNew 拒绝启动。
func migrate(db *sql.DB, driver Driver, log Logger) (from, to int, err error) {
	if log == nil {
		log = nopLogger{}
	}
	if _, err := db.Exec(createMigrationsTable(driver)); err != nil {
		return 0, 0, err
	}

	from, err = currentDBVersion(db)
	if err != nil {
		return 0, 0, err
	}
	latest, err := LatestMigrationVersion(driver)
	if err != nil {
		return 0, 0, err
	}
	if from > latest {
		return from, from, fmt.Errorf("%w: 数据库 schema 版本为 %d，当前程序仅支持到 %d；请升级程序版本",
			ErrDBTooNew, from, latest)
	}

	applied := map[int]bool{}
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return 0, 0, err
		}
		applied[v] = true
	}
	rows.Close()

	names, err := migrationFiles(driver)
	if err != nil {
		return 0, 0, err
	}

	to = from
	for _, name := range names {
		version, err := parseVersion(name)
		if err != nil {
			return from, to, err
		}
		if applied[version] {
			continue
		}
		log.Infof("正在执行数据库迁移 %s", name)
		root, sub := migrationFSFor(driver)
		content, err := fs.ReadFile(root, sub+"/"+name)
		if err != nil {
			return from, to, err
		}
		tx, err := db.Begin()
		if err != nil {
			return from, to, err
		}
		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return from, to, fmt.Errorf("执行迁移 %s 失败: %w", name, err)
		}
		insertVersion := "INSERT INTO schema_migrations(version) VALUES(?)"
		if driver == DriverPostgres {
			insertVersion = "INSERT INTO schema_migrations(version) VALUES($1)"
		}
		if _, err := tx.Exec(insertVersion, version); err != nil {
			_ = tx.Rollback()
			return from, to, fmt.Errorf("记录迁移版本 %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return from, to, err
		}
		to = version
	}
	return from, to, nil
}

func createMigrationsTable(driver Driver) string {
	switch driver {
	case DriverPostgres:
		return `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`
	case DriverMySQL:
		return `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
	default:
		return `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`
	}
}
