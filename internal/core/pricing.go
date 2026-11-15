package core

import (
	"time"
)

const (
	BillingDay  = "day"
	BillingHour = "hour"
)

// EquipmentItem is one kind of extra equipment in a rental (one-time price).
type EquipmentItem struct {
	Name      string
	Qty       int64
	UnitPrice int64 // net, per piece, one-time
}

// QuoteInput is everything needed to price a rental.
type QuoteInput struct {
	VehicleName      string
	Start, End       time.Time // End is the billable end (planned or later return)
	Billing          string    // BillingDay or BillingHour
	HourRate         int64     // net
	DayRate          int64     // net
	KmRate           int64     // net per km over the limit
	KmLimit          *int64    // nil = no limit
	KmDriven         *int64    // nil = not returned yet
	Equipment        []EquipmentItem
	VATRate          int // basis points
	ToleranceMinutes int // grace period for the last started day
}

// Line is one row of the settlement.
type Line struct {
	Name      string
	Qty       int64
	Unit      string
	UnitPrice int64 // net
	Net       int64
}

// Quote is the priced rental.
type Quote struct {
	Lines   []Line
	Units   int64 // billed days or hours
	ExtraKm int64
	Net     int64
	VAT     int64
	Gross   int64
	VATRate int
}

// BillableUnits returns the number of billed days or hours. Every started
// hour counts. Every started day counts, except that the last day is not
// charged if it was started by no more than toleranceMinutes. Minimum 1.
func BillableUnits(start, end time.Time, billing string, toleranceMinutes int) int64 {
	mins := int64(end.Sub(start) / time.Minute)
	if mins <= 0 {
		return 1
	}
	if billing == BillingHour {
		return ceilDiv(mins, 60)
	}
	days := ceilDiv(mins, 24*60)
	if days > 1 && mins-(days-1)*24*60 <= int64(toleranceMinutes) {
		days--
	}
	return days
}

func ceilDiv(a, b int64) int64 { return (a + b - 1) / b }

// ComputeQuote prices a rental.
func ComputeQuote(in QuoteInput) Quote {
	q := Quote{VATRate: in.VATRate}
	q.Units = BillableUnits(in.Start, in.End, in.Billing, in.ToleranceMinutes)
	rate, unit := in.DayRate, UnitLabel(BillingDay, q.Units)
	if in.Billing == BillingHour {
		rate, unit = in.HourRate, UnitLabel(BillingHour, q.Units)
	}
	name := "Najem pojazdu"
	if in.VehicleName != "" {
		name += " " + in.VehicleName
	}
	q.Lines = append(q.Lines, Line{Name: name, Qty: q.Units, Unit: unit, UnitPrice: rate, Net: rate * q.Units})
	for _, e := range in.Equipment {
		if e.Qty <= 0 {
			continue
		}
		q.Lines = append(q.Lines, Line{Name: e.Name, Qty: e.Qty, Unit: "szt.", UnitPrice: e.UnitPrice, Net: e.UnitPrice * e.Qty})
	}
	if in.KmLimit != nil && in.KmDriven != nil && *in.KmDriven > *in.KmLimit {
		q.ExtraKm = *in.KmDriven - *in.KmLimit
		q.Lines = append(q.Lines, Line{Name: "Przekroczenie limitu kilometrów", Qty: q.ExtraKm, Unit: "km", UnitPrice: in.KmRate, Net: in.KmRate * q.ExtraKm})
	}
	for _, l := range q.Lines {
		q.Net += l.Net
	}
	q.VAT = VAT(q.Net, in.VATRate)
	q.Gross = q.Net + q.VAT
	return q
}

// UnitLabel returns the Polish unit name for a count: 1 doba, 2 doby, 5 dób.
func UnitLabel(billing string, n int64) string {
	forms := [3]string{"doba", "doby", "dób"}
	if billing == BillingHour {
		forms = [3]string{"godzina", "godziny", "godzin"}
	}
	return Plural(n, forms)
}

// Plural picks the Polish plural form for n: [1, 2-4, 5+].
func Plural(n int64, forms [3]string) string {
	if n < 0 {
		n = -n
	}
	switch {
	case n == 1:
		return forms[0]
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return forms[1]
	default:
		return forms[2]
	}
}
