package web

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"vehicle-rent-app/internal/core"
	"vehicle-rent-app/internal/store"
)

func deleteMsg(err error, what string) string {
	if errors.Is(err, store.ErrInUse) {
		return "Nie można usunąć – " + what + " występuje w wypożyczeniach."
	}
	return "Błąd usuwania: " + err.Error()
}

// ---------- customers ----------

func (s *Server) customerList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	list, err := s.st.Customers(q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "customers.html", D{"Title": "Klienci", "Customers": list, "Q": q, "Partial": "customer-rows"})
}

func (s *Server) customerShow(w http.ResponseWriter, r *http.Request) {
	c, err := s.st.Customer(pathID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rentals, err := s.st.Rentals(store.RentalFilter{CustomerID: c.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var total int64
	for _, rt := range rentals {
		if rt.Status != store.StatusCancelled {
			total += rt.Quote().Gross
		}
	}
	s.render(w, r, "customer.html", D{"Title": c.Name(), "C": c, "Rentals": rentals, "Total": total,
		"LicenseExpired": c.LicenseExpiry != "" && c.LicenseExpiry < core.Today().Format(core.DateLayout)})
}

func (s *Server) customerForm(w http.ResponseWriter, r *http.Request) {
	c := store.Customer{Kind: "person"}
	if id := pathID(r); id != 0 {
		var err error
		if c, err = s.st.Customer(id); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.render(w, r, "customer_form.html", D{"Title": "Klient", "C": c, "Errors": map[string]string{}})
}

func readCustomer(f *form, c *store.Customer) {
	c.Kind = f.str("kind")
	if c.Kind != "company" {
		c.Kind = "person"
	}
	c.FirstName = f.str("first_name")
	c.LastName = f.str("last_name")
	c.CompanyName = f.str("company_name")
	c.NIP = f.str("nip")
	c.PESEL = f.str("pesel")
	c.IDDocument = f.str("id_document")
	c.LicenseNumber = f.str("license_number")
	c.LicenseExpiry = f.date("license_expiry")
	c.Phone = f.str("phone")
	c.Email = f.str("email")
	c.Address = f.str("address")
	c.Notes = f.str("notes")
	if c.Kind == "company" && c.CompanyName == "" {
		f.Errors["company_name"] = "Podaj nazwę firmy."
	}
	if c.Kind == "person" && c.LastName == "" {
		f.Errors["last_name"] = "Podaj nazwisko."
	}
}

func (s *Server) customerSave(w http.ResponseWriter, r *http.Request) {
	f := newForm(r)
	c := store.Customer{ID: pathID(r)}
	if c.ID != 0 {
		var err error
		if c, err = s.st.Customer(c.ID); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	readCustomer(f, &c)
	if !f.ok() {
		s.render(w, r, "customer_form.html", D{"Title": "Klient", "C": c, "Errors": f.Errors})
		return
	}
	if err := s.st.SaveCustomer(&c); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/customers/"+strconv.FormatInt(c.ID, 10), "Zapisano klienta.")
}

func (s *Server) customerDelete(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if err := s.st.DeleteCustomer(id); err != nil {
		redirect(w, r, "/customers/"+strconv.FormatInt(id, 10), deleteMsg(err, "klient"))
		return
	}
	redirect(w, r, "/customers", "Usunięto klienta.")
}

// ---------- vehicles ----------

func (s *Server) vehicleList(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.Vehicles(false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// what each vehicle is doing right now
	now := core.FormatDT(core.Now())
	soon := core.FormatDT(core.Now().Add(time.Minute))
	state := map[int64]string{}
	for _, v := range list {
		cf, _ := s.st.VehicleConflicts(v.ID, now, soon, 0, 0)
		for _, c := range cf {
			if c.Kind == "block" {
				state[v.ID] = "blocked"
			} else if state[v.ID] == "" {
				state[v.ID] = "busy"
			}
		}
	}
	s.render(w, r, "vehicles.html", D{"Title": "Pojazdy", "Vehicles": list, "State": state, "Today": core.Today().Format(core.DateLayout)})
}

func (s *Server) vehicleShow(w http.ResponseWriter, r *http.Request) {
	v, err := s.st.Vehicle(pathID(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rentals, _ := s.st.Rentals(store.RentalFilter{VehicleID: v.ID, Limit: 50})
	blocks, _ := s.st.Blocks(v.ID)
	cat, _ := s.st.PriceCategory(v.CategoryID)
	now := core.Now()
	s.render(w, r, "vehicle.html", D{"Title": v.Name(), "V": v, "Cat": cat, "Rentals": rentals, "Blocks": blocks,
		"BlockStart": core.FormatDT(now), "BlockEnd": core.FormatDT(now.AddDate(0, 0, 1)),
		"Today": core.Today().Format(core.DateLayout)})
}

func (s *Server) vehicleForm(w http.ResponseWriter, r *http.Request) {
	v := store.Vehicle{Active: true, Kind: store.VehicleKinds[0]}
	if id := pathID(r); id != 0 {
		var err error
		if v, err = s.st.Vehicle(id); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.vehicleFormRender(w, r, v, map[string]string{})
}

func (s *Server) vehicleFormRender(w http.ResponseWriter, r *http.Request, v store.Vehicle, errs map[string]string) {
	cats, err := s.st.PriceCategories()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "vehicle_form.html", D{"Title": "Pojazd", "V": v, "Categories": cats, "Errors": errs})
}

func (s *Server) vehicleSave(w http.ResponseWriter, r *http.Request) {
	f := newForm(r)
	v := store.Vehicle{ID: pathID(r)}
	v.Kind = f.required("kind", "Typ")
	v.Make = f.required("make", "Marka")
	v.Model = f.str("model")
	v.RegNumber = f.str("reg_number")
	v.VIN = f.str("vin")
	v.Year = int(f.int("year"))
	v.Color = f.str("color")
	v.Mileage = f.int("mileage")
	v.CategoryID = f.id("category_id")
	if v.CategoryID == 0 {
		f.Errors["category_id"] = "Wybierz cennik. Jeśli lista jest pusta, dodaj najpierw cennik w zakładce „Cennik”."
	}
	v.InspectionDue = f.date("inspection_due")
	v.OCDue = f.date("oc_due")
	v.ACDue = f.date("ac_due")
	v.Active = f.bool("active")
	v.Notes = f.str("notes")
	if !f.ok() {
		s.vehicleFormRender(w, r, v, f.Errors)
		return
	}
	if err := s.st.SaveVehicle(&v); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/vehicles/"+strconv.FormatInt(v.ID, 10), "Zapisano pojazd.")
}

func (s *Server) vehicleDelete(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if err := s.st.DeleteVehicle(id); err != nil {
		redirect(w, r, "/vehicles/"+strconv.FormatInt(id, 10), deleteMsg(err, "pojazd")+" Możesz go oznaczyć jako nieaktywny (odznacz „Pojazd aktywny” w edycji).")
		return
	}
	redirect(w, r, "/vehicles", "Usunięto pojazd.")
}

func (s *Server) blockAdd(w http.ResponseWriter, r *http.Request) {
	f := newForm(r)
	id := pathID(r)
	back := "/vehicles/" + strconv.FormatInt(id, 10)
	b := store.Block{VehicleID: id, StartAt: f.dt("start_at", "od"), EndAt: f.dt("end_at", "do"), Reason: f.str("reason")}
	if b.Reason == "" {
		b.Reason = "Serwis"
	}
	if f.ok() && b.EndAt <= b.StartAt {
		f.Errors["end_at"] = "Koniec musi być po początku."
	}
	if !f.ok() {
		for _, e := range f.Errors {
			redirect(w, r, back, e)
			return
		}
	}
	msg := "Dodano blokadę."
	if cf, _ := s.st.VehicleConflicts(id, b.StartAt, b.EndAt, 0, 0); len(cf) > 0 {
		for _, c := range cf {
			if c.Kind == "rental" {
				msg = "Dodano blokadę. Uwaga: w tym czasie jest wypożyczenie " + c.Label + " – przenieś je na inny pojazd."
				break
			}
		}
	}
	if err := s.st.AddBlock(&b); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, back, msg)
}

func (s *Server) blockDelete(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	if err := s.st.DeleteBlock(pathID(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	back := r.PostFormValue("back")
	if back == "" {
		back = "/vehicles"
	}
	redirect(w, r, back, "Usunięto blokadę.")
}

// ---------- equipment ----------

func (s *Server) equipmentList(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.EquipmentList(false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := core.Now()
	free, _ := s.st.EquipmentAvailable(core.FormatDT(now), core.FormatDT(now.Add(time.Minute)), 0)
	s.render(w, r, "equipment.html", D{"Title": "Wyposażenie", "Items": list, "Free": free})
}

func (s *Server) equipmentForm(w http.ResponseWriter, r *http.Request) {
	e := store.Equipment{Active: true, Stock: 1}
	if id := pathID(r); id != 0 {
		var err error
		if e, err = s.st.Equipment(id); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.render(w, r, "equipment_form.html", D{"Title": "Wyposażenie", "E": e, "Errors": map[string]string{}})
}

func (s *Server) equipmentSave(w http.ResponseWriter, r *http.Request) {
	f := newForm(r)
	e := store.Equipment{ID: pathID(r), Name: f.required("name", "Nazwa"), Stock: f.int("stock"), Price: f.money("price"), Active: f.bool("active"), Notes: f.str("notes")}
	if !f.ok() {
		s.render(w, r, "equipment_form.html", D{"Title": "Wyposażenie", "E": e, "Errors": f.Errors})
		return
	}
	if err := s.st.SaveEquipment(&e); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/equipment", "Zapisano wyposażenie.")
}

func (s *Server) equipmentDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteEquipment(pathID(r)); err != nil {
		redirect(w, r, "/equipment", deleteMsg(err, "to wyposażenie")+" Możesz je oznaczyć jako nieaktywne.")
		return
	}
	redirect(w, r, "/equipment", "Usunięto wyposażenie.")
}

// ---------- price categories ----------

func (s *Server) priceList(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.PriceCategories()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	st, _ := s.st.Settings()
	s.render(w, r, "prices.html", D{"Title": "Cennik", "Categories": list, "S": st})
}

func (s *Server) priceForm(w http.ResponseWriter, r *http.Request) {
	var c store.PriceCategory
	if id := pathID(r); id != 0 {
		var err error
		if c, err = s.st.PriceCategory(id); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.render(w, r, "price_form.html", D{"Title": "Cennik", "C": c, "Errors": map[string]string{}})
}

func (s *Server) priceSave(w http.ResponseWriter, r *http.Request) {
	f := newForm(r)
	c := store.PriceCategory{ID: pathID(r), Name: f.required("name", "Nazwa"), HourRate: f.money("hour_rate"), DayRate: f.money("day_rate"),
		KmRate: f.money("km_rate"), Deposit: f.money("deposit"), Notes: f.str("notes")}
	if !f.ok() {
		s.render(w, r, "price_form.html", D{"Title": "Cennik", "C": c, "Errors": f.Errors})
		return
	}
	if err := s.st.SavePriceCategory(&c); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/prices", "Zapisano cennik. Nowe ceny dotyczą rezerwacji; wydane i zakończone wypożyczenia zachowują swoje ceny.")
}

func (s *Server) priceDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeletePriceCategory(pathID(r)); err != nil {
		msg := "Błąd usuwania: " + err.Error()
		if errors.Is(err, store.ErrInUse) {
			msg = "Nie można usunąć – ten cennik jest przypisany do pojazdów."
		}
		redirect(w, r, "/prices", msg)
		return
	}
	redirect(w, r, "/prices", "Usunięto cennik.")
}

// ---------- settings ----------

func (s *Server) settingsForm(w http.ResponseWriter, r *http.Request) {
	st, err := s.st.Settings()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, "settings.html", D{"Title": "Ustawienia", "S": st, "Errors": map[string]string{}, "DataDir": s.opt.DataDir, "BackupDir": s.opt.BackupDir})
}

func (s *Server) settingsSave(w http.ResponseWriter, r *http.Request) {
	f := newForm(r)
	st, _ := s.st.Settings()
	st.CompanyName = f.required("company_name", "Nazwa firmy")
	st.CompanyAddress = f.str("company_address")
	st.CompanyNIP = f.str("company_nip")
	st.CompanyPhone = f.str("company_phone")
	st.CompanyEmail = f.str("company_email")
	st.BankAccount = f.str("bank_account")
	st.ContractCity = f.str("contract_city")
	st.ContractTerms = f.r.PostFormValue("contract_terms")
	if v, err := core.ParseRate(f.str("vat_rate")); err != nil {
		f.Errors["vat_rate"] = err.Error()
	} else {
		st.VATRate = v
	}
	st.ToleranceMinutes = int(f.int("tolerance_minutes"))
	st.BufferMinutes = int(f.int("buffer_minutes"))
	st.ReminderDays = int(f.int("reminder_days"))
	st.NumberPrefix = f.required("number_prefix", "Przedrostek numeru umowy")
	if !f.ok() {
		s.render(w, r, "settings.html", D{"Title": "Ustawienia", "S": st, "Errors": f.Errors, "DataDir": s.opt.DataDir, "BackupDir": s.opt.BackupDir})
		return
	}
	if err := s.st.SaveSettings(st); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/settings", "Zapisano ustawienia.")
}

func (s *Server) settingsBackup(w http.ResponseWriter, r *http.Request) {
	if s.opt.Backup == nil {
		redirect(w, r, "/settings", "Kopie zapasowe nie są dostępne.")
		return
	}
	p, err := s.opt.Backup()
	if err != nil {
		redirect(w, r, "/settings", "Nie udało się zrobić kopii: "+err.Error())
		return
	}
	redirect(w, r, "/settings", "Zrobiono kopię zapasową: "+p)
}

func (s *Server) settingsOpenFolder(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	dir := s.opt.DataDir
	if r.PostFormValue("which") == "backup" {
		dir = s.opt.BackupDir
	}
	if s.opt.OpenFolder != nil {
		if err := s.opt.OpenFolder(dir); err != nil {
			redirect(w, r, "/settings", "Nie udało się otworzyć folderu: "+err.Error())
			return
		}
	}
	redirect(w, r, "/settings", "")
}
