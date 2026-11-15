// Package store reads and writes the application data.
package store

import (
	"strings"

	"vehicle-rent-app/internal/core"
)

// Rental statuses.
const (
	StatusReserved  = "reserved"
	StatusIssued    = "issued"
	StatusReturned  = "returned"
	StatusCancelled = "cancelled"
)

// VehicleKinds are the vehicle types offered in forms.
var VehicleKinds = []string{"Samochód osobowy", "Samochód dostawczy", "Bus", "Quad", "Skuter", "Motocykl", "Przyczepa", "Inny"}

type PriceCategory struct {
	ID       int64
	Name     string
	HourRate int64
	DayRate  int64
	KmRate   int64
	Deposit  int64
	Notes    string
	Vehicles int // number of vehicles using it (lists only)
}

type Vehicle struct {
	ID            int64
	Kind          string
	Make          string
	Model         string
	RegNumber     string
	VIN           string
	Year          int
	Color         string
	Mileage       int64
	CategoryID    int64
	CategoryName  string
	InspectionDue string
	OCDue         string
	ACDue         string
	Active        bool
	Notes         string
}

// Name is "Toyota Yaris (WE 12345)".
func (v Vehicle) Name() string {
	n := strings.TrimSpace(v.Make + " " + v.Model)
	if v.RegNumber != "" {
		n += " (" + v.RegNumber + ")"
	}
	return n
}

type Block struct {
	ID        int64
	VehicleID int64
	Vehicle   string
	StartAt   string
	EndAt     string
	Reason    string
}

type Equipment struct {
	ID     int64
	Name   string
	Stock  int64
	Price  int64
	Active bool
	Notes  string
}

type Customer struct {
	ID            int64
	Kind          string // person | company
	FirstName     string
	LastName      string
	CompanyName   string
	NIP           string
	PESEL         string
	IDDocument    string
	LicenseNumber string
	LicenseExpiry string
	Phone         string
	Email         string
	Address       string
	Notes         string
	CreatedAt     string
}

// Name is the display name: company name or "Nazwisko Imię".
func (c Customer) Name() string {
	person := strings.TrimSpace(c.LastName + " " + c.FirstName)
	if c.Kind == "company" && c.CompanyName != "" {
		if person != "" {
			return c.CompanyName + " (" + person + ")"
		}
		return c.CompanyName
	}
	return person
}

type Rental struct {
	ID               int64
	Number           string
	CustomerID       int64
	CustomerName     string
	VehicleID        int64
	VehicleName      string
	StartAt          string
	EndAt            string
	Billing          string
	KmLimit          *int64
	Status           string
	HourRate         int64
	DayRate          int64
	KmRate           int64
	VATRate          int
	ToleranceMinutes int
	Deposit          int64
	DepositCollected bool
	DepositReturned  int64
	IssuedAt         string
	MileageOut       *int64
	FuelOut          string
	ConditionOut     string
	ReturnedAt       string
	MileageIn        *int64
	FuelIn           string
	ConditionIn      string
	Notes            string
	CreatedAt        string
	Equipment        []RentalEquipment
}

type RentalEquipment struct {
	EquipmentID int64
	Name        string
	Quantity    int64
	UnitPrice   int64
}

// Active reports whether the rental occupies the vehicle.
func (r Rental) Active() bool { return r.Status == StatusReserved || r.Status == StatusIssued }

// Overdue reports an issued vehicle not returned on time.
func (r Rental) Overdue() bool {
	return r.Status == StatusIssued && r.EndAt < core.FormatDT(core.Now())
}

// KmDriven is the distance driven, once returned.
func (r Rental) KmDriven() *int64 {
	if r.MileageIn == nil || r.MileageOut == nil {
		return nil
	}
	d := *r.MileageIn - *r.MileageOut
	return &d
}

// BillableEnd is the planned end, or the actual return if it was later.
func (r Rental) BillableEnd() string {
	if r.ReturnedAt != "" && r.ReturnedAt > r.EndAt {
		return r.ReturnedAt
	}
	return r.EndAt
}

// Quote prices the rental from its stored (frozen) rates.
func (r Rental) Quote() core.Quote {
	start, _ := core.ParseDT(r.StartAt)
	end, _ := core.ParseDT(r.BillableEnd())
	in := core.QuoteInput{
		VehicleName: r.VehicleName, Start: start, End: end, Billing: r.Billing,
		HourRate: r.HourRate, DayRate: r.DayRate, KmRate: r.KmRate,
		KmLimit: r.KmLimit, KmDriven: r.KmDriven(),
		VATRate: r.VATRate, ToleranceMinutes: r.ToleranceMinutes,
	}
	for _, e := range r.Equipment {
		in.Equipment = append(in.Equipment, core.EquipmentItem{Name: e.Name, Qty: e.Quantity, UnitPrice: e.UnitPrice})
	}
	return core.ComputeQuote(in)
}

// DepositKept is the part of the deposit not given back.
func (r Rental) DepositKept() int64 {
	if !r.DepositCollected || r.Status != StatusReturned {
		return 0
	}
	return r.Deposit - r.DepositReturned
}
