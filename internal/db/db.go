// Package db opens the SQLite database, applies migrations and makes backups.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure Go SQLite driver
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open opens (creating if needed) the database and migrates it.
func Open(path string) (*sql.DB, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: a single-user desktop app, and it keeps transactions simple.
	d.SetMaxOpenConns(1)
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, fmt.Errorf("nie można otworzyć bazy %s: %w", path, err)
	}
	if err := migrate(d); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func migrate(d *sql.DB) error {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var current int
	if err := d.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	for _, name := range names {
		num, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("zła nazwa migracji %s", name)
		}
		if num <= current {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := d.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migracja %s: %w", name, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", num)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Backup writes a consistent copy of the database into dir and keeps only the
// newest `keep` backups. Returns the path of the new backup.
func Backup(d *sql.DB, dir string, keep int) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := "kopia-" + time.Now().Format("2006-01-02_15-04-05")
	path := filepath.Join(dir, base+".db")
	for i := 2; fileExists(path); i++ {
		path = filepath.Join(dir, fmt.Sprintf("%s_%d.db", base, i))
	}
	if _, err := d.Exec("VACUUM INTO ?", path); err != nil {
		return "", fmt.Errorf("kopia zapasowa: %w", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "kopia-*.db"))
	sort.Strings(matches)
	for len(matches) > keep {
		os.Remove(matches[0])
		matches = matches[1:]
	}
	return path, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
