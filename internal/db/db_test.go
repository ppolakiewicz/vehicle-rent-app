package db

import (
	"database/sql"
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
	if v != 2 {
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

func TestMigrationKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	// Build a database exactly as version 1 of the program left it, with data.
	raw, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	v1, _ := migrations.ReadFile("migrations/0001_init.sql")
	for _, q := range []string{string(v1), "PRAGMA user_version = 1",
		"INSERT INTO price_categories(id, name, day_rate) VALUES(1, 'Osobowe', 15000)",
		"INSERT INTO vehicles(id, kind, make, category_id) VALUES(1, 'Quad', 'CF Moto', 1)",
		"INSERT INTO customers(id, last_name, created_at) VALUES(1, 'Nowak', '2026-01-01T10:00')",
		"INSERT INTO rentals(number, customer_id, vehicle_id, start_at, end_at, created_at) VALUES('UM/2026/0001', 1, 1, '2026-01-01T10:00', '2026-01-02T10:00', '2026-01-01T10:00')",
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(q[:min(len(q), 40)], err)
		}
	}
	raw.Close()

	d, err := Open(path) // applies 0002
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	d.QueryRow("SELECT COUNT(*) FROM rentals r JOIN vehicles v ON v.id = r.vehicle_id WHERE v.category_id = 1").Scan(&n)
	if n != 1 {
		t.Fatalf("data lost in migration")
	}
	// a vehicle without a price list is allowed now
	if _, err := d.Exec("INSERT INTO vehicles(kind, make) VALUES('Bus', 'Ford')"); err != nil {
		t.Fatal(err)
	}
	// deleting a price list unlinks its vehicles
	if _, err := d.Exec("DELETE FROM price_categories WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	var cat any
	d.QueryRow("SELECT category_id FROM vehicles WHERE id = 1").Scan(&cat)
	if cat != nil {
		t.Errorf("category not unlinked: %v", cat)
	}
	// foreign keys are enforced again after the migration
	if _, err := d.Exec("INSERT INTO rentals(number, customer_id, vehicle_id, start_at, end_at, created_at) VALUES('X', 1, 99, '', '', '')"); err == nil {
		t.Error("foreign keys not enforced")
	}
}
