// Package web serves the user interface (HTML pages rendered on the server).
package web

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"vehicle-rent-app/internal/core"
	"vehicle-rent-app/internal/store"
)

//go:embed templates static
var assets embed.FS

// Options configure the server.
type Options struct {
	DataDir    string
	BackupDir  string
	Backup     func() (string, error) // makes a backup now
	OpenFolder func(path string) error
	Version    string
}

// Server is the HTTP handler of the application.
type Server struct {
	st       *store.Store
	opt      Options
	pages    map[string]*template.Template
	mux      *http.ServeMux
	lastBeat atomic.Int64 // unix seconds of the last heartbeat from a window
}

func New(st *store.Store, opt Options) (*Server, error) {
	s := &Server{st: st, opt: opt, mux: http.NewServeMux(), pages: map[string]*template.Template{}}
	if err := s.parseTemplates(); err != nil {
		return nil, err
	}
	s.routes()
	return s, nil
}

// LastHeartbeat returns when a window last reported it is open (zero if never).
func (s *Server) LastHeartbeat() time.Time {
	v := s.lastBeat.Load()
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}

func (s *Server) parseTemplates() error {
	pages, err := fs.Glob(assets, "templates/pages/*.html")
	if err != nil {
		return err
	}
	for _, p := range pages {
		name := strings.TrimPrefix(p, "templates/pages/")
		t, err := template.New(name).Funcs(funcs).ParseFS(assets, "templates/base.html", "templates/partials/*.html", p)
		if err != nil {
			return fmt.Errorf("szablon %s: %w", name, err)
		}
		s.pages[name] = t
	}
	prints, _ := fs.Glob(assets, "templates/print/*.html")
	for _, p := range prints {
		if strings.HasSuffix(p, "/layout.html") {
			continue
		}
		name := "print/" + strings.TrimPrefix(p, "templates/print/")
		t, err := template.New(name).Funcs(funcs).ParseFS(assets, "templates/print/layout.html", p)
		if err != nil {
			return fmt.Errorf("szablon %s: %w", name, err)
		}
		s.pages[name] = t
	}
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only answer requests addressed to this computer (protects against DNS
	// rebinding) and only accept forms sent from our own pages (CSRF).
	host := r.Host
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	if host != "127.0.0.1" && host != "localhost" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = r.Header.Get("Referer")
		}
		if origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("panic %s %s: %v", r.Method, r.URL, rec)
			http.Error(w, "Wystąpił błąd programu.", http.StatusInternalServerError)
		}
	}()
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	m := s.mux
	static, _ := fs.Sub(assets, "static")
	m.Handle("GET /static/", http.StripPrefix("/static/", cacheFor(http.FileServer(http.FS(static)))))
	m.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, AppID) })
	m.HandleFunc("POST /api/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		s.lastBeat.Store(time.Now().Unix())
		w.WriteHeader(http.StatusNoContent)
	})

	m.HandleFunc("GET /{$}", s.dashboard)
	m.HandleFunc("GET /calendar", s.calendar)
	m.HandleFunc("GET /api/calendar", s.calendarData)
	m.HandleFunc("GET /search", s.search)

	m.HandleFunc("GET /rentals", s.rentalList)
	m.HandleFunc("GET /rentals/new", s.rentalNew)
	m.HandleFunc("POST /rentals", s.rentalSave)
	m.HandleFunc("POST /rentals/quote", s.rentalQuote)
	m.HandleFunc("GET /rentals/{id}", s.rentalShow)
	m.HandleFunc("GET /rentals/{id}/edit", s.rentalEdit)
	m.HandleFunc("POST /rentals/{id}", s.rentalSave)
	m.HandleFunc("GET /rentals/{id}/issue", s.rentalIssueForm)
	m.HandleFunc("POST /rentals/{id}/issue", s.rentalIssue)
	m.HandleFunc("GET /rentals/{id}/return", s.rentalReturnForm)
	m.HandleFunc("POST /rentals/{id}/return", s.rentalReturn)
	m.HandleFunc("POST /rentals/{id}/cancel", s.rentalCancel)
	m.HandleFunc("POST /rentals/{id}/restore", s.rentalRestore)
	m.HandleFunc("GET /rentals/{id}/contract", s.rentalPrint("print/contract.html"))
	m.HandleFunc("GET /rentals/{id}/protocol", s.rentalPrint("print/protocol.html"))

	m.HandleFunc("GET /customers", s.customerList)
	m.HandleFunc("GET /customers/new", s.customerForm)
	m.HandleFunc("POST /customers", s.customerSave)
	m.HandleFunc("GET /customers/{id}", s.customerShow)
	m.HandleFunc("GET /customers/{id}/edit", s.customerForm)
	m.HandleFunc("POST /customers/{id}", s.customerSave)
	m.HandleFunc("POST /customers/{id}/delete", s.customerDelete)

	m.HandleFunc("GET /vehicles", s.vehicleList)
	m.HandleFunc("GET /vehicles/new", s.vehicleForm)
	m.HandleFunc("POST /vehicles", s.vehicleSave)
	m.HandleFunc("GET /vehicles/{id}", s.vehicleShow)
	m.HandleFunc("GET /vehicles/{id}/edit", s.vehicleForm)
	m.HandleFunc("POST /vehicles/{id}", s.vehicleSave)
	m.HandleFunc("POST /vehicles/{id}/delete", s.vehicleDelete)
	m.HandleFunc("POST /vehicles/{id}/blocks", s.blockAdd)
	m.HandleFunc("POST /blocks/{id}/delete", s.blockDelete)

	m.HandleFunc("GET /equipment", s.equipmentList)
	m.HandleFunc("GET /equipment/new", s.equipmentForm)
	m.HandleFunc("POST /equipment", s.equipmentSave)
	m.HandleFunc("GET /equipment/{id}/edit", s.equipmentForm)
	m.HandleFunc("POST /equipment/{id}", s.equipmentSave)
	m.HandleFunc("POST /equipment/{id}/delete", s.equipmentDelete)

	m.HandleFunc("GET /prices", s.priceList)
	m.HandleFunc("GET /prices/new", s.priceForm)
	m.HandleFunc("POST /prices", s.priceSave)
	m.HandleFunc("GET /prices/{id}/edit", s.priceForm)
	m.HandleFunc("POST /prices/{id}", s.priceSave)
	m.HandleFunc("POST /prices/{id}/delete", s.priceDelete)

	m.HandleFunc("GET /settings", s.settingsForm)
	m.HandleFunc("POST /settings", s.settingsSave)
	m.HandleFunc("POST /settings/backup", s.settingsBackup)
	m.HandleFunc("POST /settings/open-folder", s.settingsOpenFolder)
}

// AppID is returned by /api/ping so a second start can find the running one.
const AppID = "vehicle-rent-app"

func cacheFor(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		h.ServeHTTP(w, r)
	})
}

// ---------- rendering helpers ----------

// D is the data passed to templates.
type D map[string]any

func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, data D) {
	t, ok := s.pages[page]
	if !ok {
		http.Error(w, "brak szablonu "+page, http.StatusInternalServerError)
		return
	}
	if data == nil {
		data = D{}
	}
	data["Path"] = r.URL.Path
	data["Version"] = s.opt.Version
	if c, err := r.Cookie("flash"); err == nil {
		if v, err := url.QueryUnescape(c.Value); err == nil {
			data["Flash"] = v
		}
		http.SetCookie(w, &http.Cookie{Name: "flash", Path: "/", MaxAge: -1})
	}
	name := "base"
	if strings.HasPrefix(page, "print/") {
		name = "print"
	}
	if r.Header.Get("HX-Request") == "true" && data["Partial"] != nil {
		name = data["Partial"].(string)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}

func (s *Server) renderPartial(w http.ResponseWriter, page, name string, data D) {
	t := s.pages[page]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s/%s: %v", page, name, err)
	}
}

func flash(w http.ResponseWriter, msg string) {
	http.SetCookie(w, &http.Cookie{Name: "flash", Value: url.QueryEscape(msg), Path: "/", MaxAge: 60})
}

func redirect(w http.ResponseWriter, r *http.Request, to, msg string) {
	if msg != "" {
		flash(w, msg)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "Nie znaleziono.", http.StatusNotFound)
		return
	}
	log.Printf("error %s %s: %v", r.Method, r.URL, err)
	http.Error(w, "Błąd: "+err.Error(), http.StatusInternalServerError)
}

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

// ---------- form parsing ----------

// form reads and validates form fields, collecting errors per field.
type form struct {
	r      *http.Request
	Errors map[string]string
}

func newForm(r *http.Request) *form {
	r.ParseForm()
	return &form{r: r, Errors: map[string]string{}}
}

func (f *form) str(name string) string { return strings.TrimSpace(f.r.PostFormValue(name)) }

func (f *form) required(name, label string) string {
	v := f.str(name)
	if v == "" {
		f.Errors[name] = "Pole „" + label + "” jest wymagane."
	}
	return v
}

func (f *form) bool(name string) bool {
	v := f.r.PostFormValue(name)
	return v == "1" || v == "on" || v == "true"
}

func (f *form) money(name string) int64 {
	v, err := core.ParseMoney(f.str(name))
	if err != nil {
		f.Errors[name] = "Nieprawidłowa kwota (np. 120,50)."
	}
	if v < 0 {
		f.Errors[name] = "Kwota nie może być ujemna."
	}
	return v
}

func (f *form) int(name string) int64 {
	v := strings.ReplaceAll(f.str(name), " ", "")
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		f.Errors[name] = "Wpisz liczbę całkowitą."
	}
	return n
}

func (f *form) optInt(name string) *int64 {
	if strings.TrimSpace(f.str(name)) == "" {
		return nil
	}
	n := f.int(name)
	return &n
}

func (f *form) id(name string) int64 {
	n, _ := strconv.ParseInt(f.str(name), 10, 64)
	return n
}

func (f *form) dt(name, label string) string {
	v := f.str(name)
	if v == "" {
		f.Errors[name] = "Podaj datę i godzinę: " + label + "."
		return ""
	}
	t, err := core.ParseDT(v)
	if err != nil {
		f.Errors[name] = "Nieprawidłowa data: " + label + "."
		return v
	}
	return core.FormatDT(t)
}

func (f *form) date(name string) string {
	v := f.str(name)
	if v == "" {
		return ""
	}
	if _, err := core.ParseDate(v); err != nil {
		f.Errors[name] = "Nieprawidłowa data."
	}
	return v
}

func (f *form) ok() bool { return len(f.Errors) == 0 }
