package web

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"vehicle-rent-app/internal/core"
	"vehicle-rent-app/internal/store"
)

// rentalInput is the rental form as typed by the user (for re-rendering).
type rentalInput struct {
	ID         int64
	Number     string
	Status     string
	CustomerID int64
	NewKind    string
	NewFirst   string
	NewLast    string
	NewCompany string
	NewPhone   string
	VehicleID  int64
	StartAt    string
	EndAt      string
	Billing    string
	KmLimit    string
	Notes      string
	Qty        map[int64]int64
}

// Locked reports fields that cannot change once the vehicle is issued.
func (in rentalInput) Locked() bool { return in.Status == store.StatusIssued }

func (s *Server) rentalList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.RentalFilter{Status: q.Get("status"), Search: q.Get("q"), Limit: 500}
	if f.Status == "" && q.Get("q") == "" && !q.Has("status") {
		f.Status = "active"
	}
	list, err := s.st.Rentals(f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "rentals.html", D{"Title": "Wypożyczenia", "Rentals": list, "Status": f.Status, "Q": f.Search, "Partial": "rental-rows"})
}

func (s *Server) rentalNew(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	start := core.Now().Truncate(time.Hour).Add(time.Hour)
	in := rentalInput{Billing: core.BillingDay, StartAt: core.FormatDT(start), EndAt: core.FormatDT(start.AddDate(0, 0, 1)), Qty: map[int64]int64{}}
	if v := q.Get("start"); v != "" {
		if t, err := core.ParseDT(v); err == nil {
			in.StartAt = core.FormatDT(t)
			in.EndAt = core.FormatDT(t.AddDate(0, 0, 1))
		}
	}
	if v := q.Get("end"); v != "" {
		if t, err := core.ParseDT(v); err == nil {
			in.EndAt = core.FormatDT(t)
		}
	}
	in.VehicleID, _ = strconv.ParseInt(q.Get("vehicle"), 10, 64)
	in.CustomerID, _ = strconv.ParseInt(q.Get("customer"), 10, 64)
	if q.Get("billing") == core.BillingHour {
		in.Billing = core.BillingHour
	}
	s.rentalFormRender(w, r, in, nil)
}

func (s *Server) rentalEdit(w http.ResponseWriter, r *http.Request) {
	rt, err := s.st.Rental(pathID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !rt.Active() {
		redirect(w, r, "/rentals/"+strconv.FormatInt(rt.ID, 10), "Zakończonego lub anulowanego wypożyczenia nie można edytować.")
		return
	}
	in := rentalInput{ID: rt.ID, Number: rt.Number, Status: rt.Status, CustomerID: rt.CustomerID, VehicleID: rt.VehicleID,
		StartAt: rt.StartAt, EndAt: rt.EndAt, Billing: rt.Billing, Notes: rt.Notes, Qty: map[int64]int64{}}
	if rt.KmLimit != nil {
		in.KmLimit = strconv.FormatInt(*rt.KmLimit, 10)
	}
	for _, e := range rt.Equipment {
		in.Qty[e.EquipmentID] = e.Quantity
	}
	s.rentalFormRender(w, r, in, nil)
}

func (s *Server) rentalFormRender(w http.ResponseWriter, r *http.Request, in rentalInput, errs map[string]string) {
	customers, err := s.st.Customers("")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	vehicles, err := s.st.Vehicles(true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	equipment, err := s.st.EquipmentList(true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if errs == nil {
		errs = map[string]string{}
	}
	title := "Nowe wypożyczenie"
	if in.ID != 0 {
		title = "Wypożyczenie " + in.Number
	}
	data := D{"Title": title, "In": in, "Customers": customers, "Vehicles": vehicles, "Equipment": equipment, "Errors": errs}
	s.addQuote(data, in)
	s.render(w, r, "rental_form.html", data)
}

// readRentalInput reads the posted rental form.
func readRentalInput(f *form) rentalInput {
	in := rentalInput{
		ID: f.id("id"), CustomerID: f.id("customer_id"), VehicleID: f.id("vehicle_id"),
		NewKind: f.str("new_kind"), NewFirst: f.str("new_first"), NewLast: f.str("new_last"), NewCompany: f.str("new_company"), NewPhone: f.str("new_phone"),
		StartAt: f.str("start_at"), EndAt: f.str("end_at"), Billing: f.str("billing"), KmLimit: f.str("km_limit"), Notes: f.str("notes"),
		Qty: map[int64]int64{},
	}
	if in.Billing != core.BillingHour {
		in.Billing = core.BillingDay
	}
	for k := range f.r.PostForm {
		if len(k) > 3 && k[:3] == "eq_" {
			id, err := strconv.ParseInt(k[3:], 10, 64)
			if err != nil {
				continue
			}
			if n := f.int(k); n > 0 {
				in.Qty[id] = n
			}
		}
	}
	return in
}

// buildRental turns the input into a rental with current prices, validating it.
// Returns the rental, field errors, conflicts and equipment shortages.
func (s *Server) buildRental(in rentalInput, f *form) (store.Rental, []store.Conflict, []store.EquipmentShortage, error) {
	st, err := s.st.Settings()
	if err != nil {
		return store.Rental{}, nil, nil, err
	}
	rt := store.Rental{Status: store.StatusReserved}
	if in.ID != 0 {
		if rt, err = s.st.Rental(in.ID); err != nil {
			return rt, nil, nil, err
		}
	}
	errs := f.Errors
	issued := rt.Status == store.StatusIssued
	if !issued {
		rt.CustomerID = in.CustomerID
		rt.VehicleID = in.VehicleID
		rt.Billing = in.Billing
		if t, err := core.ParseDT(in.StartAt); err != nil {
			errs["start_at"] = "Podaj datę i godzinę rozpoczęcia."
		} else {
			rt.StartAt = core.FormatDT(t)
		}
	}
	if t, err := core.ParseDT(in.EndAt); err != nil {
		errs["end_at"] = "Podaj datę i godzinę zakończenia."
	} else {
		rt.EndAt = core.FormatDT(t)
	}
	if rt.StartAt != "" && rt.EndAt != "" && rt.EndAt <= rt.StartAt {
		errs["end_at"] = "Zakończenie musi być później niż rozpoczęcie."
	}
	rt.KmLimit = nil
	if in.KmLimit != "" {
		n, err := strconv.ParseInt(in.KmLimit, 10, 64)
		if err != nil || n <= 0 {
			errs["km_limit"] = "Limit km musi być liczbą większą od zera (albo zostaw puste – bez limitu)."
		} else {
			rt.KmLimit = &n
		}
	}
	rt.Notes = in.Notes
	if rt.CustomerID == 0 && in.NewLast == "" && in.NewCompany == "" {
		errs["customer_id"] = "Wybierz klienta z listy albo wpisz dane nowego klienta."
	}
	if rt.VehicleID == 0 {
		errs["vehicle_id"] = "Wybierz pojazd."
	}

	var vehicle store.Vehicle
	if rt.VehicleID != 0 {
		if vehicle, err = s.st.Vehicle(rt.VehicleID); err != nil {
			errs["vehicle_id"] = "Nie ma takiego pojazdu."
		} else {
			rt.VehicleName = vehicle.Name()
		}
	}
	// Prices follow the price list until the vehicle is issued.
	if !issued && vehicle.ID != 0 {
		cat, err := s.st.PriceCategory(vehicle.CategoryID)
		if err != nil {
			return rt, nil, nil, err
		}
		rt.HourRate, rt.DayRate, rt.KmRate, rt.Deposit = cat.HourRate, cat.DayRate, cat.KmRate, cat.Deposit
		rt.VATRate, rt.ToleranceMinutes = st.VATRate, st.ToleranceMinutes
	}
	old := map[int64]int64{}
	for _, e := range rt.Equipment {
		old[e.EquipmentID] = e.UnitPrice
	}
	rt.Equipment = nil
	eqList, err := s.st.EquipmentList(false)
	if err != nil {
		return rt, nil, nil, err
	}
	for _, e := range eqList {
		q := in.Qty[e.ID]
		if q <= 0 {
			continue
		}
		price := e.Price
		if p, ok := old[e.ID]; ok && issued {
			price = p
		}
		rt.Equipment = append(rt.Equipment, store.RentalEquipment{EquipmentID: e.ID, Name: e.Name, Quantity: q, UnitPrice: price})
	}

	var conflicts []store.Conflict
	var shortages []store.EquipmentShortage
	if rt.VehicleID != 0 && rt.StartAt != "" && rt.EndAt > rt.StartAt {
		if conflicts, err = s.st.VehicleConflicts(rt.VehicleID, rt.StartAt, rt.EndAt, st.BufferMinutes, rt.ID); err != nil {
			return rt, nil, nil, err
		}
		free, err := s.st.EquipmentAvailable(rt.StartAt, rt.EndAt, rt.ID)
		if err != nil {
			return rt, nil, nil, err
		}
		for _, e := range rt.Equipment {
			if e.Quantity > free[e.EquipmentID] {
				shortages = append(shortages, store.EquipmentShortage{Name: e.Name, Requested: e.Quantity, Available: max(free[e.EquipmentID], 0)})
			}
		}
	}
	return rt, conflicts, shortages, nil
}

// addQuote fills the price preview for the form.
func (s *Server) addQuote(data D, in rentalInput) {
	f := &form{Errors: map[string]string{}}
	rt, conflicts, shortages, err := s.buildRental(in, f)
	if err != nil {
		data["QuoteError"] = err.Error()
		return
	}
	data["Conflicts"] = conflicts
	data["Shortages"] = shortages
	if rt.VehicleID != 0 && rt.StartAt != "" && rt.EndAt > rt.StartAt {
		q := rt.Quote()
		data["Quote"] = &q
		data["R"] = rt
	}
}

func (s *Server) rentalQuote(w http.ResponseWriter, r *http.Request) {
	f := newForm(r)
	in := readRentalInput(f)
	data := D{}
	s.addQuote(data, in)
	s.renderPartial(w, "rental_form.html", "quote", data)
}

func (s *Server) rentalSave(w http.ResponseWriter, r *http.Request) {
	f := newForm(r)
	in := readRentalInput(f)
	if id := pathID(r); id != 0 {
		in.ID = id
	}
	if in.ID != 0 {
		existing, err := s.st.Rental(in.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !existing.Active() {
			redirect(w, r, "/rentals/"+strconv.FormatInt(in.ID, 10), "Tego wypożyczenia nie można już zmieniać.")
			return
		}
		in.Status, in.Number = existing.Status, existing.Number
	}
	rt, conflicts, shortages, err := s.buildRental(in, f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(conflicts) > 0 {
		f.Errors["vehicle_id"] = "Pojazd jest zajęty w tym terminie – zmień termin albo pojazd."
	}
	if len(shortages) > 0 {
		f.Errors["equipment"] = "Brakuje wyposażenia w tym terminie."
	}
	if in.CustomerID == 0 && rt.Status == store.StatusReserved {
		if in.NewKind == "company" && in.NewCompany == "" {
			f.Errors["new_company"] = "Podaj nazwę firmy."
		}
		if in.NewKind != "company" && in.NewLast == "" {
			f.Errors["new_last"] = "Podaj nazwisko nowego klienta."
		}
	}
	if !f.ok() {
		s.rentalFormRender(w, r, in, f.Errors)
		return
	}
	if rt.CustomerID == 0 {
		c := store.Customer{Kind: "person", FirstName: in.NewFirst, LastName: in.NewLast, CompanyName: in.NewCompany, Phone: in.NewPhone}
		if in.NewKind == "company" {
			c.Kind = "company"
		}
		if err := s.st.SaveCustomer(&c); err != nil {
			s.fail(w, r, err)
			return
		}
		rt.CustomerID = c.ID
	}
	st, _ := s.st.Settings()
	isNew := rt.ID == 0
	if err := s.st.SaveRental(&rt, st.NumberPrefix); err != nil {
		s.fail(w, r, err)
		return
	}
	msg := "Zapisano zmiany."
	if isNew {
		msg = "Utworzono rezerwację " + rt.Number + "."
	}
	redirect(w, r, "/rentals/"+strconv.FormatInt(rt.ID, 10), msg)
}

func (s *Server) rentalShow(w http.ResponseWriter, r *http.Request) {
	rt, err := s.st.Rental(pathID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c, _ := s.st.Customer(rt.CustomerID)
	v, _ := s.st.Vehicle(rt.VehicleID)
	q := rt.Quote()
	s.render(w, r, "rental.html", D{"Title": "Wypożyczenie " + rt.Number, "R": rt, "C": c, "V": v, "Quote": q,
		"LicenseExpired": c.LicenseExpiry != "" && c.LicenseExpiry < rt.EndAt[:10]})
}

var fuelLevels = []string{"Pełny", "3/4", "1/2", "1/4", "Rezerwa"}

func (s *Server) rentalIssueForm(w http.ResponseWriter, r *http.Request) {
	rt, err := s.st.Rental(pathID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if rt.Status != store.StatusReserved {
		redirect(w, r, "/rentals/"+strconv.FormatInt(rt.ID, 10), "Pojazd został już wydany albo rezerwacja jest anulowana.")
		return
	}
	v, _ := s.st.Vehicle(rt.VehicleID)
	c, _ := s.st.Customer(rt.CustomerID)
	rt.IssuedAt = core.FormatDT(core.Now())
	m := v.Mileage
	rt.MileageOut = &m
	rt.FuelOut = fuelLevels[0]
	rt.DepositCollected = rt.Deposit > 0
	s.render(w, r, "rental_issue.html", D{"Title": "Wydanie pojazdu " + rt.Number, "R": rt, "V": v, "C": c, "Fuel": fuelLevels, "Errors": map[string]string{},
		"LicenseExpired": c.LicenseExpiry != "" && c.LicenseExpiry < rt.EndAt[:10]})
}

func (s *Server) rentalIssue(w http.ResponseWriter, r *http.Request) {
	rt, err := s.st.Rental(pathID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	back := "/rentals/" + strconv.FormatInt(rt.ID, 10)
	if rt.Status != store.StatusReserved {
		redirect(w, r, back, "Pojazd został już wydany albo rezerwacja jest anulowana.")
		return
	}
	f := newForm(r)
	rt.IssuedAt = f.dt("issued_at", "wydanie")
	m := f.int("mileage_out")
	rt.MileageOut = &m
	rt.FuelOut = f.str("fuel_out")
	rt.ConditionOut = f.str("condition_out")
	rt.Deposit = f.money("deposit")
	rt.DepositCollected = f.bool("deposit_collected")
	if !f.ok() {
		v, _ := s.st.Vehicle(rt.VehicleID)
		c, _ := s.st.Customer(rt.CustomerID)
		s.render(w, r, "rental_issue.html", D{"Title": "Wydanie pojazdu " + rt.Number, "R": rt, "V": v, "C": c, "Fuel": fuelLevels, "Errors": f.Errors})
		return
	}
	// make sure the vehicle is not still out with someone else
	if cf, _ := s.st.VehicleConflicts(rt.VehicleID, rt.StartAt, rt.EndAt, 0, rt.ID); len(cf) > 0 {
		redirect(w, r, back, "Nie można wydać – pojazd jest w tym czasie zajęty: "+cf[0].Label+".")
		return
	}
	rt.Status = store.StatusIssued
	if err := s.st.SaveRental(&rt, ""); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, back, "Pojazd wydany. Możesz wydrukować umowę i protokół wydania.")
}

func (s *Server) rentalReturnForm(w http.ResponseWriter, r *http.Request) {
	rt, err := s.st.Rental(pathID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if rt.Status != store.StatusIssued && rt.Status != store.StatusReturned {
		redirect(w, r, "/rentals/"+strconv.FormatInt(rt.ID, 10), "Zwrot jest możliwy tylko dla wydanego pojazdu.")
		return
	}
	if rt.Status == store.StatusIssued {
		rt.ReturnedAt = core.FormatDT(core.Now())
		rt.FuelIn = rt.FuelOut
		if rt.DepositCollected {
			rt.DepositReturned = rt.Deposit
		}
	}
	s.returnRender(w, r, rt, map[string]string{})
}

func (s *Server) returnRender(w http.ResponseWriter, r *http.Request, rt store.Rental, errs map[string]string) {
	v, _ := s.st.Vehicle(rt.VehicleID)
	q := rt.Quote()
	s.render(w, r, "rental_return.html", D{"Title": "Zwrot pojazdu " + rt.Number, "R": rt, "V": v, "Quote": q, "Fuel": fuelLevels, "Errors": errs})
}

func (s *Server) rentalReturn(w http.ResponseWriter, r *http.Request) {
	rt, err := s.st.Rental(pathID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	back := "/rentals/" + strconv.FormatInt(rt.ID, 10)
	if rt.Status != store.StatusIssued && rt.Status != store.StatusReturned {
		redirect(w, r, back, "Zwrot jest możliwy tylko dla wydanego pojazdu.")
		return
	}
	f := newForm(r)
	rt.ReturnedAt = f.dt("returned_at", "zwrot")
	rt.MileageIn = f.optInt("mileage_in")
	rt.FuelIn = f.str("fuel_in")
	rt.ConditionIn = f.str("condition_in")
	rt.DepositReturned = f.money("deposit_returned")
	if rt.MileageIn == nil {
		f.Errors["mileage_in"] = "Podaj stan licznika przy zwrocie."
	} else if rt.MileageOut != nil && *rt.MileageIn < *rt.MileageOut {
		f.Errors["mileage_in"] = "Stan licznika nie może być mniejszy niż przy wydaniu (" + strconv.FormatInt(*rt.MileageOut, 10) + " km)."
	}
	if rt.DepositReturned > rt.Deposit {
		f.Errors["deposit_returned"] = "Zwracana kwota jest większa niż kaucja."
	}
	if f.ok() && rt.ReturnedAt < rt.IssuedAt {
		f.Errors["returned_at"] = "Zwrot nie może być przed wydaniem."
	}
	if r.PostFormValue("preview") == "1" {
		q := rt.Quote()
		s.renderPartial(w, "rental_return.html", "settlement", D{"R": rt, "Quote": q, "Errors": f.Errors})
		return
	}
	if !f.ok() {
		s.returnRender(w, r, rt, f.Errors)
		return
	}
	rt.Status = store.StatusReturned
	if err := s.st.SaveRental(&rt, ""); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, back, "Zwrot zapisany. Do zapłaty: "+core.FormatMoney(rt.Quote().Gross)+".")
}

func (s *Server) rentalCancel(w http.ResponseWriter, r *http.Request) {
	rt, err := s.st.Rental(pathID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	back := "/rentals/" + strconv.FormatInt(rt.ID, 10)
	if rt.Status != store.StatusReserved {
		redirect(w, r, back, "Anulować można tylko rezerwację (pojazd jeszcze nie wydany).")
		return
	}
	rt.Status = store.StatusCancelled
	if err := s.st.SaveRental(&rt, ""); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, back, "Rezerwacja anulowana.")
}

func (s *Server) rentalRestore(w http.ResponseWriter, r *http.Request) {
	rt, err := s.st.Rental(pathID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	back := "/rentals/" + strconv.FormatInt(rt.ID, 10)
	if rt.Status != store.StatusCancelled {
		redirect(w, r, back, "")
		return
	}
	st, _ := s.st.Settings()
	if cf, _ := s.st.VehicleConflicts(rt.VehicleID, rt.StartAt, rt.EndAt, st.BufferMinutes, rt.ID); len(cf) > 0 {
		redirect(w, r, back, "Nie można przywrócić – pojazd jest już zajęty w tym terminie ("+cf[0].Label+").")
		return
	}
	rt.Status = store.StatusReserved
	if err := s.st.SaveRental(&rt, ""); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, back, "Rezerwacja przywrócona.")
}

func (s *Server) rentalPrint(page string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rt, err := s.st.Rental(pathID(r))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		c, _ := s.st.Customer(rt.CustomerID)
		v, _ := s.st.Vehicle(rt.VehicleID)
		st, _ := s.st.Settings()
		q := rt.Quote()
		s.render(w, r, page, D{"R": rt, "C": c, "V": v, "S": st, "Quote": q, "Today": core.Today().Format(core.DateLayout),
			"Kind": r.URL.Query().Get("kind")})
	}
}

// newRentalURL builds the link to a prefilled rental form.
func newRentalURL(vehicleID int64, start, end string) string {
	v := url.Values{}
	v.Set("vehicle", strconv.FormatInt(vehicleID, 10))
	v.Set("start", start)
	v.Set("end", end)
	return "/rentals/new?" + v.Encode()
}
