package core

import (
	"testing"
	"time"
)

func dt(s string) time.Time {
	t, err := ParseDT(s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseFormatMoney(t *testing.T) {
	cases := map[string]int64{"": 0, "12": 1200, "12,5": 1250, "1 234,56": 123456, "0.99": 99, "100 zł": 10000, "-3,10": -310}
	for in, want := range cases {
		got, err := ParseMoney(in)
		if err != nil || got != want {
			t.Errorf("ParseMoney(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"abc", "1,234", "1.2.3"} {
		if _, err := ParseMoney(bad); err == nil {
			t.Errorf("ParseMoney(%q) should fail", bad)
		}
	}
	if got := FormatAmount(123456789); got != "1 234 567,89" {
		t.Errorf("FormatAmount = %q", got)
	}
	if got := FormatAmountInput(1205); got != "12,05" {
		t.Errorf("FormatAmountInput = %q", got)
	}
}

func TestVAT(t *testing.T) {
	if v := VAT(10000, 2300); v != 2300 {
		t.Errorf("VAT = %d", v)
	}
	if v := VAT(1, 2300); v != 0 { // 0.23 gr
		t.Errorf("VAT small = %d", v)
	}
	if v := VAT(3, 2300); v != 1 { // 0.69 gr
		t.Errorf("VAT round = %d", v)
	}
	if r, _ := ParseRate("8,5"); r != 850 || FormatRate(850) != "8,5" {
		t.Errorf("rate = %d %s", r, FormatRate(850))
	}
}

func TestBillableUnits(t *testing.T) {
	s := dt("2026-11-15T10:00")
	cases := []struct {
		end     string
		billing string
		tol     int
		want    int64
	}{
		{"2026-11-15T10:30", BillingHour, 0, 1},
		{"2026-11-15T11:00", BillingHour, 0, 1},
		{"2026-11-15T11:01", BillingHour, 60, 2},
		{"2026-11-15T13:00", BillingHour, 0, 3},
		{"2026-11-15T12:00", BillingDay, 60, 1},
		{"2026-11-16T10:00", BillingDay, 60, 1},
		{"2026-11-16T11:00", BillingDay, 60, 1}, // within tolerance
		{"2026-11-16T11:01", BillingDay, 60, 2},
		{"2026-11-16T10:01", BillingDay, 0, 2},
		{"2026-11-18T10:00", BillingDay, 60, 3},
		{"2026-11-15T10:00", BillingDay, 60, 1}, // zero length is still 1
	}
	for _, c := range cases {
		if got := BillableUnits(s, dt(c.end), c.billing, c.tol); got != c.want {
			t.Errorf("%s %s tol %d: got %d want %d", c.end, c.billing, c.tol, got, c.want)
		}
	}
	// DST: 2026-10-25 clocks go back, but a doba is a calendar day.
	if got := BillableUnits(dt("2026-10-24T10:00"), dt("2026-10-25T10:00"), BillingDay, 0); got != 1 {
		t.Errorf("DST: got %d", got)
	}
}

func TestComputeQuote(t *testing.T) {
	limit, driven := int64(300), int64(420)
	q := ComputeQuote(QuoteInput{
		VehicleName: "Toyota Yaris",
		Start:       dt("2026-11-15T10:00"), End: dt("2026-11-18T10:30"),
		Billing: BillingDay, DayRate: 15000, HourRate: 2000, KmRate: 80,
		KmLimit: &limit, KmDriven: &driven,
		Equipment: []EquipmentItem{{Name: "Fotelik", Qty: 2, UnitPrice: 3000}, {Name: "Bagażnik", Qty: 0, UnitPrice: 5000}},
		VATRate:   2300, ToleranceMinutes: 60,
	})
	if q.Units != 3 || len(q.Lines) != 3 || q.ExtraKm != 120 {
		t.Fatalf("units %d lines %d extra %d", q.Units, len(q.Lines), q.ExtraKm)
	}
	// 3*150 + 2*30 + 120*0.80 = 450 + 60 + 96 = 606 zł
	if q.Net != 60600 || q.VAT != 13938 || q.Gross != 74538 {
		t.Errorf("net %d vat %d gross %d", q.Net, q.VAT, q.Gross)
	}
	if q.Lines[0].Unit != "doby" {
		t.Errorf("unit %q", q.Lines[0].Unit)
	}
	h := ComputeQuote(QuoteInput{Start: dt("2026-11-15T10:00"), End: dt("2026-11-15T12:15"), Billing: BillingHour, HourRate: 5000, VATRate: 2300})
	if h.Units != 3 || h.Net != 15000 {
		t.Errorf("hourly units %d net %d", h.Units, h.Net)
	}
}

func TestPlural(t *testing.T) {
	f := [3]string{"doba", "doby", "dób"}
	for n, want := range map[int64]string{1: "doba", 2: "doby", 4: "doby", 5: "dób", 12: "dób", 22: "doby", 25: "dób"} {
		if got := Plural(n, f); got != want {
			t.Errorf("Plural(%d) = %s", n, got)
		}
	}
}
