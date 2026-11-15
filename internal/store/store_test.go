package store

import (
	"path/filepath"
	"testing"

	"vehicle-rent-app/internal/db"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return New(d)
}

func seed(t *testing.T, s *Store) (Vehicle, Customer, Equipment) {
	t.Helper()
	cat := PriceCategory{Name: "Osobowe", DayRate: 15000, HourRate: 2500, KmRate: 80, Deposit: 100000}
	must(t, s.SavePriceCategory(&cat))
	v := Vehicle{Kind: "Samochód osobowy", Make: "Toyota", Model: "Yaris", RegNumber: "WE 12345", CategoryID: cat.ID, Active: true}
	must(t, s.SaveVehicle(&v))
	c := Customer{Kind: "person", FirstName: "Jan", LastName: "Kowalski"}
	must(t, s.SaveCustomer(&c))
	e := Equipment{Name: "Fotelik", Stock: 2, Price: 3000, Active: true}
	must(t, s.SaveEquipment(&e))
	return v, c, e
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestRentalAvailability(t *testing.T) {
	s := newStore(t)
	v, c, e := seed(t, s)
	r := Rental{CustomerID: c.ID, VehicleID: v.ID, StartAt: "2030-05-01T10:00", EndAt: "2030-05-03T10:00", Billing: "day", Status: StatusReserved,
		DayRate: 15000, VATRate: 2300, Equipment: []RentalEquipment{{EquipmentID: e.ID, Quantity: 2, UnitPrice: 3000}}}
	must(t, s.SaveRental(&r, "UM"))
	if r.Number != "UM/2030/0001" {
		t.Errorf("number %s", r.Number)
	}

	cases := []struct {
		start, end string
		buffer     int
		conflict   bool
	}{
		{"2030-04-30T10:00", "2030-05-01T10:00", 0, false}, // ends when the other starts
		{"2030-05-03T10:00", "2030-05-04T10:00", 0, false}, // starts when the other ends
		{"2030-05-03T10:00", "2030-05-04T10:00", 60, true}, // buffer
		{"2030-05-02T00:00", "2030-05-02T12:00", 0, true},
		{"2030-04-01T00:00", "2030-06-01T00:00", 0, true},
	}
	for _, cs := range cases {
		cf, err := s.VehicleConflicts(v.ID, cs.start, cs.end, cs.buffer, 0)
		must(t, err)
		if (len(cf) > 0) != cs.conflict {
			t.Errorf("%s-%s buffer %d: conflicts %v", cs.start, cs.end, cs.buffer, cf)
		}
	}
	// editing the same rental does not conflict with itself
	cf, _ := s.VehicleConflicts(v.ID, "2030-05-02T00:00", "2030-05-02T12:00", 0, r.ID)
	if len(cf) != 0 {
		t.Errorf("self conflict %v", cf)
	}

	avail, _ := s.AvailableVehicles("2030-05-02T00:00", "2030-05-02T12:00", "", 0)
	if len(avail) != 0 {
		t.Errorf("vehicle should be busy")
	}
	avail, _ = s.AvailableVehicles("2030-06-02T00:00", "2030-06-02T12:00", "Quad", 0)
	if len(avail) != 0 {
		t.Errorf("kind filter")
	}

	eq, _ := s.EquipmentAvailable("2030-05-02T00:00", "2030-05-02T12:00", 0)
	if eq[e.ID] != 0 {
		t.Errorf("equipment free = %d", eq[e.ID])
	}
	eq, _ = s.EquipmentAvailable("2030-05-03T10:00", "2030-05-05T12:00", 0)
	if eq[e.ID] != 2 {
		t.Errorf("equipment free after = %d", eq[e.ID])
	}

	// cancelled rentals do not block
	r.Status = StatusCancelled
	must(t, s.SaveRental(&r, "UM"))
	cf, _ = s.VehicleConflicts(v.ID, "2030-05-02T00:00", "2030-05-02T12:00", 0, 0)
	if len(cf) != 0 {
		t.Errorf("cancelled blocks")
	}

	// service block
	must(t, s.AddBlock(&Block{VehicleID: v.ID, StartAt: "2030-07-01T08:00", EndAt: "2030-07-02T16:00", Reason: "Serwis"}))
	cf, _ = s.VehicleConflicts(v.ID, "2030-07-02T10:00", "2030-07-03T10:00", 0, 0)
	if len(cf) != 1 || cf[0].Kind != "block" {
		t.Errorf("block conflict %v", cf)
	}
}

func TestOverdueIssuedBlocksUntilNow(t *testing.T) {
	s := newStore(t)
	v, c, _ := seed(t, s)
	r := Rental{CustomerID: c.ID, VehicleID: v.ID, StartAt: "2020-01-01T10:00", EndAt: "2020-01-02T10:00", Billing: "day", Status: StatusIssued, VATRate: 2300}
	must(t, s.SaveRental(&r, "UM"))
	// a new rental starting in 2021 overlaps because the car was never returned
	cf, _ := s.VehicleConflicts(v.ID, "2021-01-01T10:00", "2021-01-02T10:00", 0, 0)
	if len(cf) != 1 {
		t.Errorf("overdue rental should block, got %v", cf)
	}
	d, err := s.Dashboard(30)
	must(t, err)
	if len(d.Overdue) != 1 {
		t.Errorf("dashboard overdue %d", len(d.Overdue))
	}
}

func TestReturnUpdatesMileageAndDelete(t *testing.T) {
	s := newStore(t)
	v, c, _ := seed(t, s)
	out, in := int64(1000), int64(1350)
	r := Rental{CustomerID: c.ID, VehicleID: v.ID, StartAt: "2026-01-01T10:00", EndAt: "2026-01-02T10:00", Billing: "day", Status: StatusReturned,
		MileageOut: &out, MileageIn: &in, VATRate: 2300}
	must(t, s.SaveRental(&r, "UM"))
	got, _ := s.Vehicle(v.ID)
	if got.Mileage != 1350 {
		t.Errorf("mileage %d", got.Mileage)
	}
	if err := s.DeleteVehicle(v.ID); err != ErrInUse {
		t.Errorf("delete vehicle with rentals: %v", err)
	}
	if err := s.DeleteCustomer(c.ID); err != ErrInUse {
		t.Errorf("delete customer with rentals: %v", err)
	}
	r2 := Rental{CustomerID: c.ID, VehicleID: v.ID, StartAt: "2026-02-01T10:00", EndAt: "2026-02-02T10:00", Billing: "day", Status: StatusReserved, VATRate: 2300}
	must(t, s.SaveRental(&r2, "UM"))
	if r2.Number != "UM/2026/0002" {
		t.Errorf("number %s", r2.Number)
	}
	list, _ := s.Customers("kowal")
	if len(list) != 1 {
		t.Errorf("search customers %d", len(list))
	}
}
