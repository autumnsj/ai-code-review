package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

var (
	//go:embed migrations/sqlite/*.sql
	sqliteMigrations embed.FS
	//go:embed migrations/postgres/*.sql
	postgresMigrations embed.FS
)

func migrationFSFor(driver Driver) (fs.FS, string) {
	if driver == DriverPostgres {
		return postgresMigrations, "migrations/postgres"
	}
	return sqliteMigrations, "migrations/sqlite"
}

func migrate(db *sql.DB, driver Driver) error {
	if _, err := db.Exec(createMigrationsTable(driver)); err != nil {
		return err
	}

	applied := map[int]bool{}
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	root, sub := migrationFSFor(driver)
	entries, err := fs.ReadDir(root, sub)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var version int
		if _, err := fmt.Sscanf(name, "%d_", &version); err != nil {
			return fmt.Errorf("parse migration version %q: %w", name, err)
		}
		if applied[version] {
			continue
		}
		content, err := fs.ReadFile(root, sub+"/"+name)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec %s: %w", name, err)
		}
		insertVersion := "INSERT INTO schema_migrations(version) VALUES(?)"
		if driver == DriverPostgres {
			insertVersion = "INSERT INTO schema_migrations(version) VALUES($1)"
		}
		if _, err := tx.Exec(insertVersion, version); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func createMigrationsTable(driver Driver) string {
	if driver == DriverPostgres {
		return `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`
	}
	return `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`
}
