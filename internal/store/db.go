package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // postgres 驱动（纯 Go）
	_ "modernc.org/sqlite"             // 纯 Go sqlite 驱动
)

// Open 按方言打开数据库、应用连接池设置并执行迁移。
//   - sqlite: dsn 为数据库文件路径（WAL 自动开启）
//   - postgres: dsn 为 PostgreSQL 连接串（目标库应已存在，可先用 EnsurePostgresDatabase 创建）
func Open(driver Driver, dsn string) (*sql.DB, error) {
	var (
		db  *sql.DB
		err error
	)
	switch driver {
	case DriverSQLite:
		db, err = openSQLite(dsn)
	case DriverPostgres:
		db, err = openPostgres(dsn)
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
	if err := migrate(db, driver); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
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
