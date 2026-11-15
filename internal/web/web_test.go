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
		{"/rentals", url.Values{"customer_id": {"1"}, "vehicle_id": {"1"}, "start_at": {"2030-01-01T10:00"}, "end_at": {"2030-01-02T10:00"}, "billing": {"day"}, "km_limit": {"200"}, "eq_1": {"1"}}, 303},
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
