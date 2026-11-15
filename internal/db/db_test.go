package db

import (
	"path/filepath"
	"testing"
)

func TestOpenMigrateBackup(t *testing.T) {
	dir := t.TempDir()
	// Polish characters in the path, like "Dokumenty\Wypożyczalnia".
	path := filepath.Join(dir, "Wypożyczalnia ąę.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var v int
	d.QueryRow("PRAGMA user_version").Scan(&v)
	if v != 1 {
		t.Fatalf("user_version = %d", v)
	}
	var fk int
	d.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if fk != 1 {
		t.Fatal("foreign keys off")
	}
	for i := 0; i < 3; i++ {
		if _, err := Backup(d, filepath.Join(dir, "kopie"), 2); err != nil {
			t.Fatal(err)
		}
	}
	m, _ := filepath.Glob(filepath.Join(dir, "kopie", "*.db"))
	if len(m) > 2 {
		t.Fatalf("kept %d backups", len(m))
	}
	d.Close()
	// reopen: migrations must not run again
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
}
