package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// Driver 标识底层数据库方言。
type Driver string

const (
	DriverSQLite   Driver = "sqlite"
	DriverPostgres Driver = "postgres"
)

// Store 聚合所有数据访问方法，持有唯一的 *sql.DB 与方言标识。
type Store struct {
	db  *sql.DB
	drv Driver
}

func New(db *sql.DB, drv Driver) *Store {
	return &Store{db: db, drv: drv}
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Driver() Driver { return s.drv }

// now 返回数据库方言的当前时间戳表达式。
func (s *Store) now() string {
	if s.drv == DriverPostgres {
		return "NOW()"
	}
	return "datetime('now')"
}

// rebind 将查询中的 '?' 占位符转换为方言风格。
// sqlite 使用 '?'；postgres 使用 '$1','$2',...
func (s *Store) rebind(q string) string {
	if s.drv != DriverPostgres {
		return q
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

// insertID 执行 INSERT 并返回自增主键。
// sqlite 依赖 LastInsertId；postgres 通过追加 RETURNING id 取回。
func (s *Store) insertID(ctx context.Context, query string, args ...any) (int64, error) {
	if s.drv == DriverPostgres {
		var id int64
		q := s.rebind(query)
		if !strings.Contains(strings.ToUpper(q), "RETURNING") {
			q = strings.TrimRight(strings.TrimSpace(q), ";") + " RETURNING id"
		}
		if err := s.db.QueryRowContext(ctx, q, args...).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	res, err := s.db.ExecContext(ctx, s.rebind(query), args...)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}
