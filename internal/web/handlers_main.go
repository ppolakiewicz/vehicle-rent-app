package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"vehicle-rent-app/internal/core"
	"vehicle-rent-app/internal/store"
)

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	st, err := s.st.Settings()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d, err := s.st.Dashboard(st.ReminderDays)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cats, _ := s.st.PriceCategories()
	s.render(w, r, "dashboard.html", D{"Title": "Pulpit", "D": d, "S": st, "NoCategories": len(cats) == 0,
		"Today": core.Today().Format(core.DateLayout)})
}

func (s *Server) calendar(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "calendar.html", D{"Title": "Kalendarz"})
}

type calGroup struct {
	ID      int64  `json:"id"`
	Content string `json:"content"`
	Title   string `json:"title"`
}

type calItem struct {
	ID        string `json:"id"`
	Group     int64  `json:"group"`
	Content   string `json:"content"`
	Title     string `json:"title"`
	Start     string `json:"start"`
	End       string `json:"end"`
	ClassName string `json:"className"`
	URL       string `json:"url"`
}

// calendarData returns vehicles and their rentals/blocks for the timeline.
func (s *Server) calendarData(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if _, err := core.ParseDT(from); err != nil {
		from = core.FormatDT(core.Today().AddDate(0, 0, -7))
	}
	if _, err := core.ParseDT(to); err != nil {
		to = core.FormatDT(core.Today().AddDate(0, 1, 0))
	}
	vehicles, err := s.st.Vehicles(true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := struct {
		Groups []calGroup `json:"groups"`
		Items  []calItem  `json:"items"`
	}{Groups: []calGroup{}, Items: []calItem{}}
	for _, v := range vehicles {
		out.Groups = append(out.Groups, calGroup{ID: v.ID, Content: v.Name(), Title: v.Kind + " · " + v.CategoryName})
	}
	rentals, err := s.st.Rentals(store.RentalFilter{From: from, To: to})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := core.FormatDT(core.Now())
	for _, rt := range rentals {
		if rt.Status == store.StatusCancelled {
			continue
		}
		end := rt.EndAt
		cls := rt.Status
		if rt.Status == store.StatusReturned && rt.ReturnedAt != "" {
			end = rt.ReturnedAt
		}
		if rt.Overdue() {
			end, cls = now, "overdue"
		}
		out.Items = append(out.Items, calItem{
			ID: "r" + strconv.FormatInt(rt.ID, 10), Group: rt.VehicleID, Content: rt.CustomerName,
			Title: rt.Number + " · " + rt.CustomerName + " · " + core.HumanDT(rt.StartAt) + " – " + core.HumanDT(rt.EndAt) + " · " + statusLabels[rt.Status],
			Start: rt.StartAt, End: end, ClassName: cls, URL: "/rentals/" + strconv.FormatInt(rt.ID, 10),
		})
	}
	blocks, err := s.st.Blocks(0)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, b := range blocks {
		if b.EndAt <= from || b.StartAt >= to {
			continue
		}
		out.Items = append(out.Items, calItem{
			ID: "b" + strconv.FormatInt(b.ID, 10), Group: b.VehicleID, Content: "⛔ " + b.Reason,
			Title: "Blokada: " + b.Reason + " · " + core.HumanDT(b.StartAt) + " – " + core.HumanDT(b.EndAt),
			Start: b.StartAt, End: b.EndAt, ClassName: "block", URL: "/vehicles/" + strconv.FormatInt(b.VehicleID, 10),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

type searchResult struct {
	V     store.Vehicle
	Quote core.Quote
	URL   string
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	start := core.Now().Truncate(time.Hour).Add(time.Hour)
	data := D{"Title": "Wolne pojazdy", "Start": core.FormatDT(start), "End": core.FormatDT(start.AddDate(0, 0, 1)), "Kind": q.Get("kind"), "Billing": q.Get("billing")}
	if data["Billing"] != core.BillingHour {
		data["Billing"] = core.BillingDay
	}
	if q.Get("start") == "" {
		s.render(w, r, "search.html", data)
		return
	}
	st, _ := s.st.Settings()
	ts, err1 := core.ParseDT(q.Get("start"))
	te, err2 := core.ParseDT(q.Get("end"))
	data["Start"], data["End"] = q.Get("start"), q.Get("end")
	if err1 != nil || err2 != nil || !te.After(ts) {
		data["Error"] = "Podaj poprawny termin – koniec musi być po początku."
		s.render(w, r, "search.html", data)
		return
	}
	startS, endS := core.FormatDT(ts), core.FormatDT(te)
	vs, err := s.st.AvailableVehicles(startS, endS, q.Get("kind"), st.BufferMinutes)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cats, _ := s.st.PriceCategories()
	byID := map[int64]store.PriceCategory{}
	for _, c := range cats {
		byID[c.ID] = c
	}
	var res []searchResult
	for _, v := range vs {
		c := byID[v.CategoryID]
		quote := core.ComputeQuote(core.QuoteInput{Start: ts, End: te, Billing: data["Billing"].(string), HourRate: c.HourRate, DayRate: c.DayRate,
			VATRate: st.VATRate, ToleranceMinutes: st.ToleranceMinutes})
		u := newRentalURL(v.ID, startS, endS)
		if data["Billing"] == core.BillingHour {
			u += "&billing=hour"
		}
		res = append(res, searchResult{V: v, Quote: quote, URL: u})
	}
	data["Results"] = res
	data["Searched"] = true
	s.render(w, r, "search.html", data)
}
