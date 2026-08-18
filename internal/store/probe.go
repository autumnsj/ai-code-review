package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Ping 仅测试能否连通数据库，不执行迁移、不保留连接。
// sqlite 的 dsn 为文件路径（文件/目录不存在时会尝试创建父目录）。
// postgres 的 dsn 为连接串；若目标数据库不存在，会尝试自动创建后再连通。
func Ping(ctx context.Context, driver Driver, dsn string) error {
	switch driver {
	case DriverSQLite:
		return pingSQLite(ctx, dsn)
	case DriverPostgres:
		return pingPostgres(ctx, dsn)
	case DriverMySQL:
		return pingMySQL(ctx, dsn)
	default:
		return fmt.Errorf("unsupported driver %q", driver)
	}
}

func pingSQLite(ctx context.Context, path string) error {
	db, err := openSQLite(path)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

func pingPostgres(ctx context.Context, dsn string) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	if err := db.PingContext(ctx); err == nil {
		db.Close()
		return nil
	} else if !isUnknownDBError(err) {
		db.Close()
		return err
	}
	db.Close()

	// 目标库不存在：连到默认 postgres 库创建它。
	if err := createPostgresDatabase(ctx, dsn); err != nil {
		return err
	}
	db2, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db2.Close()
	return db2.PingContext(ctx)
}

// pingMySQL 探测 MySQL 连通性；若目标库不存在（错误 1049 Unknown database），
// 连到无库连接并尝试 CREATE DATABASE。
func pingMySQL(ctx context.Context, dsn string) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	db, err := openMySQL(dsn)
	if err != nil {
		return err
	}
	if err := db.PingContext(ctx); err == nil {
		db.Close()
		return nil
	} else if !isUnknownMySQLError(err) {
		db.Close()
		return err
	}
	db.Close()

	if err := createMySQLDatabase(ctx, dsn); err != nil {
		return err
	}
	db2, err := openMySQL(dsn)
	if err != nil {
		return err
	}
	defer db2.Close()
	return db2.PingContext(ctx)
}

func isUnknownMySQLError(err error) bool {
	// go-sql-driver/mysql: Error 1049: Unknown database 'xxx'
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		return myErr.Number == 1049
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown database")
}

// createMySQLDatabase 解析 DSN 得到库名，用去掉库名的维护连接执行 CREATE DATABASE。
func createMySQLDatabase(ctx context.Context, dsn string) error {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return err
	}
	dbName := cfg.DBName
	if dbName == "" {
		return fmt.Errorf("dsn 中没有数据库名")
	}
	if !validDBName(dbName) {
		return fmt.Errorf("invalid database name %q", dbName)
	}
	maint := *cfg
	maint.DBName = ""
	conn, err := sql.Open("mysql", maint.FormatDSN())
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.PingContext(ctx); err != nil {
		return fmt.Errorf("connect mysql maintenance: %w", err)
	}
	// 库名不能参数化；已用 validDBName 做白名单校验。
	_, err = conn.ExecContext(ctx,
		"CREATE DATABASE IF NOT EXISTS `"+dbName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	return err
}

func isUnknownDBError(err error) bool {
	msg := strings.ToLower(err.Error())
	// pgx: "database \"xxx\" does not exist"
	return strings.Contains(msg, "does not exist") && strings.Contains(msg, "database")
}

// createPostgresDatabase 解析 DSN 中的 dbname，连到维护库 postgres 执行 CREATE DATABASE。
func createPostgresDatabase(ctx context.Context, dsn string) error {
	maintDSN, dbName, err := maintenanceDSN(dsn)
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", maintDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect maintenance db: %w", err)
	}
	// 数据库名不能参数化，且来自本机配置输入；做基本字符校验。
	if !validDBName(dbName) {
		return fmt.Errorf("invalid database name %q", dbName)
	}
	_, err = db.ExecContext(ctx, "CREATE DATABASE \""+dbName+"\"")
	return err
}

func maintenanceDSN(dsn string) (string, string, error) {
	// 支持标准 URL 形式: postgres://user:pass@host:port/dbname?params
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", "", err
		}
		dbName := strings.TrimPrefix(u.Path, "/")
		u.Path = "/postgres"
		return u.String(), dbName, nil
	}
	// 关键字形式: host=... dbname=... user=...
	fields := strings.Fields(dsn)
	dbName := ""
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if strings.HasPrefix(f, "dbname=") {
			dbName = strings.TrimPrefix(f, "dbname=")
			continue
		}
		out = append(out, f)
	}
	if dbName == "" {
		return "", "", fmt.Errorf("dsn 中没有 dbname/路径数据库名")
	}
	out = append(out, "dbname=postgres")
	return strings.Join(out, " "), dbName, nil
}

func validDBName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
