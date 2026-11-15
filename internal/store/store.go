package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"vehicle-rent-app/internal/core"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

// ErrInUse is returned when deleting something other records refer to.
var ErrInUse = errors.New("rekord jest używany i nie może zostać usunięty")

var ErrNotFound = sql.ErrNoRows

func nullInt(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------- price categories ----------

func (s *Store) PriceCategories() ([]PriceCategory, error) {
	rows, err := s.db.Query(`SELECT c.id, c.name, c.hour_rate, c.day_rate, c.km_rate, c.deposit, c.notes,
		(SELECT COUNT(*) FROM vehicles v WHERE v.category_id = c.id)
		FROM price_categories c ORDER BY c.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PriceCategory
	for rows.Next() {
		var c PriceCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.HourRate, &c.DayRate, &c.KmRate, &c.Deposit, &c.Notes, &c.Vehicles); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) PriceCategory(id int64) (PriceCategory, error) {
	var c PriceCategory
	err := s.db.QueryRow(`SELECT id, name, hour_rate, day_rate, km_rate, deposit, notes FROM price_categories WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.HourRate, &c.DayRate, &c.KmRate, &c.Deposit, &c.Notes)
	return c, err
}

func (s *Store) SavePriceCategory(c *PriceCategory) error {
	if c.ID == 0 {
		res, err := s.db.Exec(`INSERT INTO price_categories(name, hour_rate, day_rate, km_rate, deposit, notes) VALUES(?,?,?,?,?,?)`,
			c.Name, c.HourRate, c.DayRate, c.KmRate, c.Deposit, c.Notes)
		if err != nil {
			return err
		}
		c.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.Exec(`UPDATE price_categories SET name=?, hour_rate=?, day_rate=?, km_rate=?, deposit=?, notes=? WHERE id=?`,
		c.Name, c.HourRate, c.DayRate, c.KmRate, c.Deposit, c.Notes, c.ID)
	return err
}

func (s *Store) DeletePriceCategory(id int64) error {
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM vehicles WHERE category_id = ?", id).Scan(&n)
	if n > 0 {
		return ErrInUse
	}
	_, err := s.db.Exec("DELETE FROM price_categories WHERE id = ?", id)
	return err
}

// ---------- vehicles ----------

const vehicleCols = `v.id, v.kind, v.make, v.model, v.reg_number, v.vin, v.year, v.color, v.mileage, v.category_id, c.name,
	v.inspection_due, v.oc_due, v.ac_due, v.active, v.notes`

func scanVehicle(sc interface{ Scan(...any) error }) (Vehicle, error) {
	var v Vehicle
	var active int
	err := sc.Scan(&v.ID, &v.Kind, &v.Make, &v.Model, &v.RegNumber, &v.VIN, &v.Year, &v.Color, &v.Mileage, &v.CategoryID, &v.CategoryName,
		&v.InspectionDue, &v.OCDue, &v.ACDue, &active, &v.Notes)
	v.Active = active == 1
	return v, err
}

// Vehicles lists vehicles; onlyActive hides withdrawn ones.
func (s *Store) Vehicles(onlyActive bool) ([]Vehicle, error) {
	q := `SELECT ` + vehicleCols + ` FROM vehicles v JOIN price_categories c ON c.id = v.category_id`
	if onlyActive {
		q += ` WHERE v.active = 1`
	}
	q += ` ORDER BY v.active DESC, v.kind, v.make COLLATE NOCASE, v.model COLLATE NOCASE, v.reg_number`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Vehicle
	for rows.Next() {
		v, err := scanVehicle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Vehicle(id int64) (Vehicle, error) {
	return scanVehicle(s.db.QueryRow(`SELECT `+vehicleCols+` FROM vehicles v JOIN price_categories c ON c.id = v.category_id WHERE v.id = ?`, id))
}

func (s *Store) SaveVehicle(v *Vehicle) error {
	args := []any{v.Kind, v.Make, v.Model, v.RegNumber, v.VIN, v.Year, v.Color, v.Mileage, v.CategoryID, v.InspectionDue, v.OCDue, v.ACDue, b2i(v.Active), v.Notes}
	if v.ID == 0 {
		res, err := s.db.Exec(`INSERT INTO vehicles(kind, make, model, reg_number, vin, year, color, mileage, category_id, inspection_due, oc_due, ac_due, active, notes)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return err
		}
		v.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.Exec(`UPDATE vehicles SET kind=?, make=?, model=?, reg_number=?, vin=?, year=?, color=?, mileage=?, category_id=?,
		inspection_due=?, oc_due=?, ac_due=?, active=?, notes=? WHERE id=?`, append(args, v.ID)...)
	return err
}

func (s *Store) DeleteVehicle(id int64) error {
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM rentals WHERE vehicle_id = ?", id).Scan(&n)
	if n > 0 {
		return ErrInUse
	}
	_, err := s.db.Exec("DELETE FROM vehicles WHERE id = ?", id)
	return err
}

// ---------- vehicle blocks (service etc.) ----------

func (s *Store) Blocks(vehicleID int64) ([]Block, error) {
	q := `SELECT b.id, b.vehicle_id, v.make || ' ' || v.model || CASE WHEN v.reg_number <> '' THEN ' (' || v.reg_number || ')' ELSE '' END,
		b.start_at, b.end_at, b.reason FROM vehicle_blocks b JOIN vehicles v ON v.id = b.vehicle_id`
	var args []any
	if vehicleID != 0 {
		q += " WHERE b.vehicle_id = ?"
		args = append(args, vehicleID)
	}
	q += " ORDER BY b.start_at DESC"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Block
	for rows.Next() {
		var b Block
		if err := rows.Scan(&b.ID, &b.VehicleID, &b.Vehicle, &b.StartAt, &b.EndAt, &b.Reason); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) AddBlock(b *Block) error {
	res, err := s.db.Exec("INSERT INTO vehicle_blocks(vehicle_id, start_at, end_at, reason) VALUES(?,?,?,?)", b.VehicleID, b.StartAt, b.EndAt, b.Reason)
	if err != nil {
		return err
	}
	b.ID, err = res.LastInsertId()
	return err
}

func (s *Store) DeleteBlock(id int64) error {
	_, err := s.db.Exec("DELETE FROM vehicle_blocks WHERE id = ?", id)
	return err
}

// ---------- equipment ----------

func (s *Store) EquipmentList(onlyActive bool) ([]Equipment, error) {
	q := "SELECT id, name, stock, price, active, notes FROM equipment"
	if onlyActive {
		q += " WHERE active = 1"
	}
	q += " ORDER BY active DESC, name COLLATE NOCASE"
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Equipment
	for rows.Next() {
		var e Equipment
		var active int
		if err := rows.Scan(&e.ID, &e.Name, &e.Stock, &e.Price, &active, &e.Notes); err != nil {
			return nil, err
		}
		e.Active = active == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) Equipment(id int64) (Equipment, error) {
	var e Equipment
	var active int
	err := s.db.QueryRow("SELECT id, name, stock, price, active, notes FROM equipment WHERE id = ?", id).
		Scan(&e.ID, &e.Name, &e.Stock, &e.Price, &active, &e.Notes)
	e.Active = active == 1
	return e, err
}

func (s *Store) SaveEquipment(e *Equipment) error {
	if e.ID == 0 {
		res, err := s.db.Exec("INSERT INTO equipment(name, stock, price, active, notes) VALUES(?,?,?,?,?)", e.Name, e.Stock, e.Price, b2i(e.Active), e.Notes)
		if err != nil {
			return err
		}
		e.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.Exec("UPDATE equipment SET name=?, stock=?, price=?, active=?, notes=? WHERE id=?", e.Name, e.Stock, e.Price, b2i(e.Active), e.Notes, e.ID)
	return err
}

func (s *Store) DeleteEquipment(id int64) error {
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM rental_equipment WHERE equipment_id = ?", id).Scan(&n)
	if n > 0 {
		return ErrInUse
	}
	_, err := s.db.Exec("DELETE FROM equipment WHERE id = ?", id)
	return err
}

// ---------- customers ----------

const customerCols = `id, kind, first_name, last_name, company_name, nip, pesel, id_document, license_number, license_expiry,
	phone, email, address, notes, created_at`

func scanCustomer(sc interface{ Scan(...any) error }) (Customer, error) {
	var c Customer
	err := sc.Scan(&c.ID, &c.Kind, &c.FirstName, &c.LastName, &c.CompanyName, &c.NIP, &c.PESEL, &c.IDDocument, &c.LicenseNumber, &c.LicenseExpiry,
		&c.Phone, &c.Email, &c.Address, &c.Notes, &c.CreatedAt)
	return c, err
}

// Customers lists customers, optionally filtered by a search text.
func (s *Store) Customers(search string) ([]Customer, error) {
	q := "SELECT " + customerCols + " FROM customers"
	var args []any
	if search = strings.TrimSpace(search); search != "" {
		q += ` WHERE (first_name || ' ' || last_name || ' ' || last_name || ' ' || first_name || ' ' || company_name || ' ' || nip || ' ' || pesel || ' ' || phone || ' ' || email || ' ' || id_document)
			LIKE ? COLLATE NOCASE`
		args = append(args, "%"+search+"%")
	}
	q += " ORDER BY CASE WHEN kind = 'company' AND company_name <> '' THEN company_name ELSE last_name || ' ' || first_name END COLLATE NOCASE"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Customer
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) Customer(id int64) (Customer, error) {
	return scanCustomer(s.db.QueryRow("SELECT "+customerCols+" FROM customers WHERE id = ?", id))
}

func (s *Store) SaveCustomer(c *Customer) error {
	args := []any{c.Kind, c.FirstName, c.LastName, c.CompanyName, c.NIP, c.PESEL, c.IDDocument, c.LicenseNumber, c.LicenseExpiry, c.Phone, c.Email, c.Address, c.Notes}
	if c.ID == 0 {
		c.CreatedAt = core.FormatDT(core.Now())
		res, err := s.db.Exec(`INSERT INTO customers(kind, first_name, last_name, company_name, nip, pesel, id_document, license_number, license_expiry,
			phone, email, address, notes, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, append(args, c.CreatedAt)...)
		if err != nil {
			return err
		}
		c.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.Exec(`UPDATE customers SET kind=?, first_name=?, last_name=?, company_name=?, nip=?, pesel=?, id_document=?, license_number=?,
		license_expiry=?, phone=?, email=?, address=?, notes=? WHERE id=?`, append(args, c.ID)...)
	return err
}

func (s *Store) DeleteCustomer(id int64) error {
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM rentals WHERE customer_id = ?", id).Scan(&n)
	if n > 0 {
		return ErrInUse
	}
	_, err := s.db.Exec("DELETE FROM customers WHERE id = ?", id)
	return err
}

// ---------- rentals ----------

const rentalCols = `r.id, r.number, r.customer_id,
	CASE WHEN cu.kind = 'company' AND cu.company_name <> '' THEN cu.company_name ELSE trim(cu.last_name || ' ' || cu.first_name) END,
	r.vehicle_id,
	trim(v.make || ' ' || v.model) || CASE WHEN v.reg_number <> '' THEN ' (' || v.reg_number || ')' ELSE '' END,
	r.start_at, r.end_at, r.billing, r.km_limit, r.status, r.hour_rate, r.day_rate, r.km_rate, r.vat_rate, r.tolerance_minutes,
	r.deposit, r.deposit_collected, r.deposit_returned, r.issued_at, r.mileage_out, r.fuel_out, r.condition_out,
	r.returned_at, r.mileage_in, r.fuel_in, r.condition_in, r.notes, r.created_at`

const rentalFrom = ` FROM rentals r JOIN customers cu ON cu.id = r.customer_id JOIN vehicles v ON v.id = r.vehicle_id`

func scanRental(sc interface{ Scan(...any) error }) (Rental, error) {
	var r Rental
	var kmLimit, mOut, mIn sql.NullInt64
	var depCollected int
	err := sc.Scan(&r.ID, &r.Number, &r.CustomerID, &r.CustomerName, &r.VehicleID, &r.VehicleName,
		&r.StartAt, &r.EndAt, &r.Billing, &kmLimit, &r.Status, &r.HourRate, &r.DayRate, &r.KmRate, &r.VATRate, &r.ToleranceMinutes,
		&r.Deposit, &depCollected, &r.DepositReturned, &r.IssuedAt, &mOut, &r.FuelOut, &r.ConditionOut,
		&r.ReturnedAt, &mIn, &r.FuelIn, &r.ConditionIn, &r.Notes, &r.CreatedAt)
	r.KmLimit, r.MileageOut, r.MileageIn = ptr(kmLimit), ptr(mOut), ptr(mIn)
	r.DepositCollected = depCollected == 1
	return r, err
}

// RentalFilter selects rentals for lists.
type RentalFilter struct {
	Status     string // "" = all, "active" = reserved+issued
	CustomerID int64
	VehicleID  int64
	Search     string
	From, To   string // overlap with period (datetime strings)
	Limit      int
}

func (s *Store) Rentals(f RentalFilter) ([]Rental, error) {
	q := "SELECT " + rentalCols + rentalFrom + " WHERE 1=1"
	var args []any
	switch f.Status {
	case "":
	case "active":
		q += " AND r.status IN ('reserved','issued')"
	default:
		q += " AND r.status = ?"
		args = append(args, f.Status)
	}
	if f.CustomerID != 0 {
		q += " AND r.customer_id = ?"
		args = append(args, f.CustomerID)
	}
	if f.VehicleID != 0 {
		q += " AND r.vehicle_id = ?"
		args = append(args, f.VehicleID)
	}
	if f.From != "" {
		q += " AND r.end_at > ?"
		args = append(args, f.From)
	}
	if f.To != "" {
		q += " AND r.start_at < ?"
		args = append(args, f.To)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		q += ` AND (r.number || ' ' || cu.first_name || ' ' || cu.last_name || ' ' || cu.company_name || ' ' || v.make || ' ' || v.model || ' ' || v.reg_number) LIKE ? COLLATE NOCASE`
		args = append(args, "%"+s+"%")
	}
	q += " ORDER BY r.start_at DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	return s.queryRentals(q, args...)
}

func (s *Store) queryRentals(q string, args ...any) ([]Rental, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rental
	for rows.Next() {
		r, err := scanRental(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Rental loads one rental with its equipment.
func (s *Store) Rental(id int64) (Rental, error) {
	r, err := scanRental(s.db.QueryRow("SELECT "+rentalCols+rentalFrom+" WHERE r.id = ?", id))
	if err != nil {
		return r, err
	}
	rows, err := s.db.Query(`SELECT re.equipment_id, e.name, re.quantity, re.unit_price FROM rental_equipment re
		JOIN equipment e ON e.id = re.equipment_id WHERE re.rental_id = ? ORDER BY e.name COLLATE NOCASE`, id)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var e RentalEquipment
		if err := rows.Scan(&e.EquipmentID, &e.Name, &e.Quantity, &e.UnitPrice); err != nil {
			return r, err
		}
		r.Equipment = append(r.Equipment, e)
	}
	return r, rows.Err()
}

// NextNumber returns the next contract number, e.g. "UM/2026/0007".
func (s *Store) NextNumber(prefix string, year int) (string, error) {
	return nextNumber(s.db, prefix, year)
}

type querier interface {
	QueryRow(query string, args ...any) *sql.Row
}

func nextNumber(s querier, prefix string, year int) (string, error) {
	if prefix == "" {
		prefix = "UM"
	}
	base := fmt.Sprintf("%s/%d/", prefix, year)
	var last sql.NullString
	err := s.QueryRow("SELECT MAX(number) FROM rentals WHERE number LIKE ? AND length(number) = ?", base+"%", len(base)+4).Scan(&last)
	if err != nil {
		return "", err
	}
	n := 1
	if last.Valid {
		fmt.Sscanf(strings.TrimPrefix(last.String, base), "%d", &n)
		n++
	}
	return fmt.Sprintf("%s%04d", base, n), nil
}

// SaveRental inserts or updates the rental and its equipment.
func (s *Store) SaveRental(r *Rental, prefix string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	args := []any{r.CustomerID, r.VehicleID, r.StartAt, r.EndAt, r.Billing, nullInt(r.KmLimit), r.Status,
		r.HourRate, r.DayRate, r.KmRate, r.VATRate, r.ToleranceMinutes, r.Deposit, b2i(r.DepositCollected), r.DepositReturned,
		r.IssuedAt, nullInt(r.MileageOut), r.FuelOut, r.ConditionOut, r.ReturnedAt, nullInt(r.MileageIn), r.FuelIn, r.ConditionIn, r.Notes}
	if r.ID == 0 {
		start, _ := core.ParseDT(r.StartAt)
		if r.Number, err = nextNumber(tx, prefix, start.Year()); err != nil {
			return err
		}
		r.CreatedAt = core.FormatDT(core.Now())
		res, err := tx.Exec(`INSERT INTO rentals(customer_id, vehicle_id, start_at, end_at, billing, km_limit, status,
			hour_rate, day_rate, km_rate, vat_rate, tolerance_minutes, deposit, deposit_collected, deposit_returned,
			issued_at, mileage_out, fuel_out, condition_out, returned_at, mileage_in, fuel_in, condition_in, notes, number, created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, append(args, r.Number, r.CreatedAt)...)
		if err != nil {
			return err
		}
		if r.ID, err = res.LastInsertId(); err != nil {
			return err
		}
	} else {
		_, err := tx.Exec(`UPDATE rentals SET customer_id=?, vehicle_id=?, start_at=?, end_at=?, billing=?, km_limit=?, status=?,
			hour_rate=?, day_rate=?, km_rate=?, vat_rate=?, tolerance_minutes=?, deposit=?, deposit_collected=?, deposit_returned=?,
			issued_at=?, mileage_out=?, fuel_out=?, condition_out=?, returned_at=?, mileage_in=?, fuel_in=?, condition_in=?, notes=?
			WHERE id=?`, append(args, r.ID)...)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM rental_equipment WHERE rental_id = ?", r.ID); err != nil {
			return err
		}
	}
	for _, e := range r.Equipment {
		if e.Quantity <= 0 {
			continue
		}
		if _, err := tx.Exec("INSERT INTO rental_equipment(rental_id, equipment_id, quantity, unit_price) VALUES(?,?,?,?)",
			r.ID, e.EquipmentID, e.Quantity, e.UnitPrice); err != nil {
			return err
		}
	}
	// keep the vehicle's odometer up to date
	if r.Status == StatusReturned && r.MileageIn != nil {
		if _, err := tx.Exec("UPDATE vehicles SET mileage = MAX(mileage, ?) WHERE id = ?", *r.MileageIn, r.VehicleID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- availability ----------

// Conflict is something that occupies a vehicle in a period.
type Conflict struct {
	Kind      string // "rental" | "block"
	ID        int64
	Label     string
	StartAt   string
	EndAt     string
	VehicleID int64
}

// occupied returns rentals and blocks of the vehicle (0 = all vehicles)
// overlapping [start, end), widened by buffer minutes. An issued rental that
// is overdue occupies the vehicle until now.
func (s *Store) occupied(vehicleID int64, start, end string, bufferMin int, excludeRental int64) ([]Conflict, error) {
	st, err := core.ParseDT(start)
	if err != nil {
		return nil, err
	}
	en, err := core.ParseDT(end)
	if err != nil {
		return nil, err
	}
	from := core.FormatDT(st.Add(-minutes(bufferMin)))
	to := core.FormatDT(en.Add(minutes(bufferMin)))
	now := core.FormatDT(core.Now())
	q := `SELECT 'rental', r.id, r.number || ' – ' || CASE WHEN cu.kind = 'company' AND cu.company_name <> '' THEN cu.company_name ELSE trim(cu.last_name || ' ' || cu.first_name) END,
			r.start_at, CASE WHEN r.status = 'issued' AND r.end_at < ?1 THEN ?1 ELSE r.end_at END AS eff_end, r.vehicle_id
		FROM rentals r JOIN customers cu ON cu.id = r.customer_id
		WHERE r.status IN ('reserved','issued') AND r.id <> ?2 AND (?3 = 0 OR r.vehicle_id = ?3)
		  AND r.start_at < ?5 AND eff_end > ?4
		UNION ALL
		SELECT 'block', b.id, 'Blokada: ' || b.reason, b.start_at, b.end_at, b.vehicle_id FROM vehicle_blocks b
		WHERE (?3 = 0 OR b.vehicle_id = ?3) AND b.start_at < ?5 AND b.end_at > ?4
		ORDER BY 4`
	rows, err := s.db.Query(q, now, excludeRental, vehicleID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Conflict
	for rows.Next() {
		var c Conflict
		if err := rows.Scan(&c.Kind, &c.ID, &c.Label, &c.StartAt, &c.EndAt, &c.VehicleID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// VehicleConflicts lists what blocks the vehicle in the period.
func (s *Store) VehicleConflicts(vehicleID int64, start, end string, bufferMin int, excludeRental int64) ([]Conflict, error) {
	return s.occupied(vehicleID, start, end, bufferMin, excludeRental)
}

// AvailableVehicles returns active vehicles free in the period (kind "" = all).
func (s *Store) AvailableVehicles(start, end, kind string, bufferMin int) ([]Vehicle, error) {
	busy, err := s.occupied(0, start, end, bufferMin, 0)
	if err != nil {
		return nil, err
	}
	taken := map[int64]bool{}
	for _, c := range busy {
		taken[c.VehicleID] = true
	}
	all, err := s.Vehicles(true)
	if err != nil {
		return nil, err
	}
	var out []Vehicle
	for _, v := range all {
		if !taken[v.ID] && (kind == "" || v.Kind == kind) {
			out = append(out, v)
		}
	}
	return out, nil
}

// EquipmentShortage describes equipment that is not available in a period.
type EquipmentShortage struct {
	Name      string
	Requested int64
	Available int64
}

// EquipmentAvailable returns how many pieces of each equipment are free in
// the whole period. It finds the peak usage over the period.
func (s *Store) EquipmentAvailable(start, end string, excludeRental int64) (map[int64]int64, error) {
	now := core.FormatDT(core.Now())
	rows, err := s.db.Query(`SELECT re.equipment_id, re.quantity, r.start_at,
			CASE WHEN r.status = 'issued' AND r.end_at < ?1 THEN ?1 ELSE r.end_at END AS eff_end
		FROM rental_equipment re JOIN rentals r ON r.id = re.rental_id
		WHERE r.status IN ('reserved','issued') AND r.id <> ?2 AND r.start_at < ?4 AND eff_end > ?3`, now, excludeRental, start, end)
	if err != nil {
		return nil, err
	}
	type use struct {
		qty        int64
		start, end string
	}
	uses := map[int64][]use{}
	for rows.Next() {
		var id int64
		var u use
		if err := rows.Scan(&id, &u.qty, &u.start, &u.end); err != nil {
			rows.Close()
			return nil, err
		}
		uses[id] = append(uses[id], u)
	}
	rows.Close()
	list, err := s.EquipmentList(false)
	if err != nil {
		return nil, err
	}
	out := map[int64]int64{}
	for _, e := range list {
		// peak concurrent usage: check at every usage start (and period start)
		var peak int64
		points := []string{start}
		for _, u := range uses[e.ID] {
			if u.start > start {
				points = append(points, u.start)
			}
		}
		for _, p := range points {
			var sum int64
			for _, u := range uses[e.ID] {
				if u.start <= p && u.end > p {
					sum += u.qty
				}
			}
			if sum > peak {
				peak = sum
			}
		}
		out[e.ID] = e.Stock - peak
	}
	return out, nil
}

// ---------- dashboard ----------

// Reminder is an upcoming or overdue vehicle date (inspection, insurance).
type Reminder struct {
	VehicleID int64
	Vehicle   string
	What      string
	Due       string
	Overdue   bool
}

// Reminders lists inspection and insurance dates within `days` days.
func (s *Store) Reminders(days int) ([]Reminder, error) {
	vs, err := s.Vehicles(true)
	if err != nil {
		return nil, err
	}
	today := core.Today()
	limit := today.AddDate(0, 0, days).Format(core.DateLayout)
	todayS := today.Format(core.DateLayout)
	var out []Reminder
	for _, v := range vs {
		for _, d := range []struct{ what, due string }{{"Przegląd techniczny", v.InspectionDue}, {"Ubezpieczenie OC", v.OCDue}, {"Ubezpieczenie AC", v.ACDue}} {
			if d.due != "" && d.due <= limit {
				out = append(out, Reminder{VehicleID: v.ID, Vehicle: v.Name(), What: d.what, Due: d.due, Overdue: d.due < todayS})
			}
		}
	}
	// soonest first
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Due < out[j-1].Due; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// Dashboard groups the rentals that need attention.
type Dashboard struct {
	PickupsToday  []Rental // reserved, start today or earlier (late pickups)
	ReturnsToday  []Rental // issued, end today
	Overdue       []Rental // issued, end in the past
	Upcoming      []Rental // reserved, next 7 days (after today)
	Reminders     []Reminder
	ActiveCount   int
	ReservedCount int
	VehicleCount  int
}

func (s *Store) Dashboard(reminderDays int) (Dashboard, error) {
	var d Dashboard
	now := core.FormatDT(core.Now())
	today := core.Today()
	tomorrow := core.FormatDT(today.AddDate(0, 0, 1))
	week := core.FormatDT(today.AddDate(0, 0, 8))
	var err error
	if d.PickupsToday, err = s.queryRentals("SELECT "+rentalCols+rentalFrom+" WHERE r.status = 'reserved' AND r.start_at < ? ORDER BY r.start_at", tomorrow); err != nil {
		return d, err
	}
	if d.ReturnsToday, err = s.queryRentals("SELECT "+rentalCols+rentalFrom+" WHERE r.status = 'issued' AND r.end_at >= ? AND r.end_at < ? ORDER BY r.end_at", now, tomorrow); err != nil {
		return d, err
	}
	if d.Overdue, err = s.queryRentals("SELECT "+rentalCols+rentalFrom+" WHERE r.status = 'issued' AND r.end_at < ? ORDER BY r.end_at", now); err != nil {
		return d, err
	}
	if d.Upcoming, err = s.queryRentals("SELECT "+rentalCols+rentalFrom+" WHERE r.status = 'reserved' AND r.start_at >= ? AND r.start_at < ? ORDER BY r.start_at", tomorrow, week); err != nil {
		return d, err
	}
	if d.Reminders, err = s.Reminders(reminderDays); err != nil {
		return d, err
	}
	s.db.QueryRow("SELECT COUNT(*) FROM rentals WHERE status = 'issued'").Scan(&d.ActiveCount)
	s.db.QueryRow("SELECT COUNT(*) FROM rentals WHERE status = 'reserved'").Scan(&d.ReservedCount)
	s.db.QueryRow("SELECT COUNT(*) FROM vehicles WHERE active = 1").Scan(&d.VehicleCount)
	return d, nil
}
