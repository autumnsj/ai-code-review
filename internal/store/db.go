package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"   // mysql 驱动（纯 Go，init 注册）
	_ "github.com/jackc/pgx/v5/stdlib" // postgres 驱动（纯 Go）
	_ "modernc.org/sqlite"             // 纯 Go sqlite 驱动
)

// Open 按方言打开数据库、应用连接池设置并执行前向迁移。
//   - sqlite: dsn 为数据库文件路径（WAL 自动开启）
//   - postgres: dsn 为 PostgreSQL 连接串（目标库应已存在，可先用 EnsurePostgresDatabase 创建）
//
// log 用于记录迁移进度与版本信息；传 nil 则静默迁移。
// 升级（存在未应用迁移）前会自动备份 SQLite 数据库文件，迁移失败可用备份回滚。
// 若数据库版本高于当前二进制内置版本，返回 ErrDBTooNew。
func Open(driver Driver, dsn string, log Logger) (*sql.DB, error) {
	var (
		db  *sql.DB
		err error
	)
	switch driver {
	case DriverSQLite:
		db, err = openSQLite(dsn)
	case DriverPostgres:
		db, err = openPostgres(dsn)
	case DriverMySQL:
		db, err = openMySQL(dsn)
	default:
		return nil, fmt.Errorf("unsupported driver %q", driver)
	}
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s: %w", driver, err)
	}

	if log == nil {
		log = nopLogger{}
	}

	// 在写任何迁移之前先确认目标版本，并为 SQLite 做升级前备份。
	latest, err := LatestMigrationVersion(driver)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if driver == DriverSQLite {
		// 先建表以便读取当前版本（建表本身是幂等的，不改变数据）。
		if _, err := db.Exec(createMigrationsTable(driver)); err != nil {
			_ = db.Close()
			return nil, err
		}
		cur, err := currentDBVersion(db)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		// cur==0 是全新库，无既有数据需要备份。
		if cur > 0 && cur < latest {
			if bak, berr := backupSQLite(db, dsn); berr != nil {
				_ = db.Close()
				return nil, fmt.Errorf("升级前备份数据库失败: %w", berr)
			} else if bak != "" {
				log.Infof("已在升级前备份数据库到 %s", bak)
			}
		}
	}

	from, to, err := migrate(db, driver, log)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	switch {
	case from == 0 && to > 0:
		log.Infof("数据库已初始化，schema 版本 %d", to)
	case from < to:
		log.Infof("数据库已升级: schema 版本 %d -> %d", from, to)
	default:
		log.Infof("数据库 schema 版本为 %d（最新）", from)
	}
	return db, nil
}

// backupSQLite 在升级前把数据库文件（含 WAL 回放后的一致快照）复制到同目录的 .bak 文件。
// 返回备份文件路径；若当前无待执行迁移或为内存库则返回空路径。
func backupSQLite(db *sql.DB, dsn string) (string, error) {
	// file:<path>?... 形式，剥离 query 与前缀。
	path := strings.TrimPrefix(dsn, "file:")
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if path == ":memory:" || path == "" {
		return "", nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		// 全新数据库，文件尚不存在，无需备份。
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	// 将 WAL 回放到主库，得到可安全复制的一致快照。
	if _, err := db.ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return "", fmt.Errorf("wal checkpoint: %w", err)
	}

	backup := fmt.Sprintf("%s.bak-%s", abs, time.Now().Format("20060102-150405"))
	src, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		_ = os.Remove(backup)
		return "", err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(backup)
		return "", err
	}
	pruneBackups(abs, 5)
	return backup, nil
}

// pruneBackups 删除多余的旧备份，仅保留最近 keep 份（按文件名时间戳排序）。
func pruneBackups(dbPath string, keep int) {
	pattern := filepath.Base(dbPath) + ".bak-*"
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(dbPath), pattern))
	if err != nil || len(matches) <= keep {
		return
	}
	sort.Strings(matches)
	for _, old := range matches[:len(matches)-keep] {
		_ = os.Remove(old)
	}
}

func openSQLite(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"+
			"&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite 单写者，单连接从根源消除 SQLITE_BUSY。WAL 下读写不互斥，低并发足够。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

func openPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	return db, nil
}

// openMySQL 打开 MySQL/MariaDB 连接。强制启用：
//   - parseTime=true：DATETIME 列扫描为 time.Time（代码里大量 Scan 到 NullTime/time.Time）
//   - multiStatements=true：迁移框架把整个 .sql 文件作为一条 Exec 执行
//   - utf8mb4 字符集（若 DSN 未指定）
func openMySQL(dsn string) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse mysql dsn: %w", err)
	}
	cfg.ParseTime = true
	cfg.MultiStatements = true
	cfg.Params = mergeParam(cfg.Params, "charset", "utf8mb4")
	cfg.Params = mergeParam(cfg.Params, "collation", "utf8mb4_unicode_ci")

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	return db, nil
}

// mergeParam 仅在 DSN 未显式设置该参数时填入默认值。
func mergeParam(params map[string]string, key, def string) map[string]string {
	if params == nil {
		params = map[string]string{}
	}
	if _, ok := params[key]; !ok {
		params[key] = def
	}
	return params
}
