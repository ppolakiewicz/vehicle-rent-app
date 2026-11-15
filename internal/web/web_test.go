package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"vehicle-rent-app/internal/db"
	"vehicle-rent-app/internal/store"
)

func newServer(t *testing.T) *Server {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s, err := New(store.New(d), Options{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func do(s *Server, method, path string, form url.Values) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, "http://127.0.0.1"+path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://127.0.0.1")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

func TestPagesAndRentalFlow(t *testing.T) {
	s := newServer(t)
	steps := []struct {
		path string
		form url.Values
		want int
	}{
		{"/prices", url.Values{"name": {"Osobowe"}, "day_rate": {"150"}, "hour_rate": {"25"}, "km_rate": {"0,80"}, "deposit": {"1000"}}, 303},
		{"/vehicles", url.Values{"kind": {"Quad"}, "make": {"CF Moto"}, "category_id": {"1"}, "active": {"1"}, "mileage": {"100"}}, 303},
		{"/equipment", url.Values{"name": {"Fotelik"}, "stock": {"1"}, "price": {"30"}, "active": {"1"}}, 303},
		{"/customers", url.Values{"kind": {"person"}, "first_name": {"Anna"}, "last_name": {"Nowak"}}, 303},
		{"/rentals", url.Values{"customer_id": {"1"}, "vehicle_id": {"1"}, "start_at": {"2030-01-01T10:00"}, "end_at": {"2030-01-02T10:00"}, "billing": {"day"}, "km_limit": {"200"}, "eq_1": {"1"},
			"day_rate": {"150"}, "km_rate": {"0,80"}, "deposit": {"1000"}, "eqp_1": {"30"}}, 303},
		// same vehicle, overlapping -> form shown again with error
		{"/rentals", url.Values{"customer_id": {"1"}, "vehicle_id": {"1"}, "start_at": {"2030-01-01T12:00"}, "end_at": {"2030-01-03T10:00"}, "billing": {"day"}}, 200},
		{"/rentals/1/issue", url.Values{"issued_at": {"2030-01-01T10:00"}, "mileage_out": {"100"}, "fuel_out": {"Pełny"}, "deposit": {"1000"}, "deposit_collected": {"1"}}, 303},
		{"/rentals/1/return", url.Values{"returned_at": {"2030-01-02T10:30"}, "mileage_in": {"350"}, "fuel_in": {"Pełny"}, "deposit_returned": {"1000"}}, 303},
		{"/settings", url.Values{"company_name": {"Firma"}, "vat_rate": {"23"}, "tolerance_minutes": {"60"}, "number_prefix": {"UM"}, "reminder_days": {"30"}}, 303},
	}
	for _, st := range steps {
		if w := do(s, "POST", st.path, st.form); w.Code != st.want {
			t.Fatalf("POST %s: %d %s", st.path, w.Code, w.Body.String())
		}
	}
	for _, p := range []string{"/", "/calendar", "/api/calendar", "/search", "/search?start=2030-02-01T10:00&end=2030-02-02T10:00", "/rentals", "/rentals?status=returned",
		"/rentals/new", "/rentals/1", "/rentals/1/contract", "/rentals/1/protocol", "/rentals/1/return",
		"/customers", "/customers/1", "/customers/1/edit", "/customers/new", "/vehicles", "/vehicles/1", "/vehicles/1/edit", "/vehicles/new",
		"/equipment", "/equipment/1/edit", "/prices", "/prices/1/edit", "/settings", "/help", "/help?q=kaucja"} {
		w := do(s, "GET", p, nil)
		if w.Code != 200 {
			t.Errorf("GET %s: %d", p, w.Code)
		}
		if strings.Contains(w.Body.String(), "ZgotmplZ") {
			t.Errorf("GET %s: unsafe template value", p)
		}
	}
	// 1 day + 30 min within tolerance = 1 day; 250 km driven, limit 200: 50 km extra
	// net 150 + 30 + 40 = 220, gross 270.60
	if body := do(s, "GET", "/rentals/1", nil).Body.String(); !strings.Contains(body, "270,60") {
		t.Errorf("expected total 270,60 in rental page")
	}
}

func TestSecurityChecks(t *testing.T) {
	s := newServer(t)
	req := httptest.NewRequest("GET", "http://evil.example/", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("foreign host: %d", w.Code)
	}
	req = httptest.NewRequest("POST", "http://127.0.0.1/customers", strings.NewReader("last_name=X"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://evil.example")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("foreign origin: %d", w.Code)
	}
}

func TestManualPrices(t *testing.T) {
	s := newServer(t)
	post := func(path string, form url.Values) {
		t.Helper()
		if w := do(s, "POST", path, form); w.Code != 303 {
			t.Fatalf("POST %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	post("/prices", url.Values{"name": {"Quady"}, "day_rate": {"300"}, "hour_rate": {"80"}, "km_rate": {"1,50"}, "deposit": {"1500"}})
	post("/vehicles", url.Values{"kind": {"Quad"}, "make": {"CF Moto"}, "category_id": {"1"}, "active": {"1"}})
	post("/vehicles", url.Values{"kind": {"Bus"}, "make": {"Ford"}, "active": {"1"}}) // no price list
	post("/customers", url.Values{"kind": {"person"}, "last_name": {"Nowak"}})

	// a new rental for a vehicle with a price list starts with its prices
	body := do(s, "GET", "/rentals/new?vehicle=1", nil).Body.String()
	if !strings.Contains(body, `name="day_rate" value="300,00"`) || !strings.Contains(body, `name="deposit" value="1500,00"`) {
		t.Error("prices not filled from the price list")
	}
	// ...and without one, the fields are empty
	body = do(s, "GET", "/rentals/new?vehicle=2", nil).Body.String()
	if !strings.Contains(body, `name="day_rate" value=""`) {
		t.Error("vehicle without price list should start with empty prices")
	}

	// typed prices win over the price list: 2 days x 99 = 198 net, 243,54 gross
	post("/rentals", url.Values{"customer_id": {"1"}, "vehicle_id": {"1"}, "start_at": {"2030-01-01T10:00"}, "end_at": {"2030-01-03T10:00"},
		"billing": {"day"}, "day_rate": {"99"}, "deposit": {"500"}})
	// a vehicle without a price list, hourly: 3 h x 50 = 150 net, 184,50 gross
	post("/rentals", url.Values{"customer_id": {"1"}, "vehicle_id": {"2"}, "start_at": {"2030-01-01T10:00"}, "end_at": {"2030-01-01T12:30"},
		"billing": {"hour"}, "hour_rate": {"50"}})
	// changing the price list does not change existing rentals
	post("/prices/1", url.Values{"name": {"Quady"}, "day_rate": {"999"}})
	if b := do(s, "GET", "/rentals/1", nil).Body.String(); !strings.Contains(b, "243,54") || !strings.Contains(b, "500,00") {
		t.Error("rental 1 should keep typed prices")
	}
	if b := do(s, "GET", "/rentals/2", nil).Body.String(); !strings.Contains(b, "184,50") {
		t.Error("rental 2 total")
	}
	// deleting the price list keeps the vehicle
	post("/prices/1/delete", url.Values{})
	if w := do(s, "GET", "/vehicles/1", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "Brak domyślnego cennika") {
		t.Errorf("vehicle page after deleting its price list: %d", w.Code)
	}
	// a bad amount is reported, not saved as zero
	w := do(s, "POST", "/rentals", url.Values{"customer_id": {"1"}, "vehicle_id": {"2"}, "start_at": {"2030-02-01T10:00"}, "end_at": {"2030-02-02T10:00"},
		"billing": {"day"}, "day_rate": {"abc"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Nieprawidłowa kwota") {
		t.Errorf("bad price accepted: %d", w.Code)
	}
}
