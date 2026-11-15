package web

import (
	"html/template"
	"strings"

	"vehicle-rent-app/internal/core"
	"vehicle-rent-app/internal/store"
)

var statusLabels = map[string]string{
	store.StatusReserved:  "Rezerwacja",
	store.StatusIssued:    "Wydany",
	store.StatusReturned:  "Zwrócony",
	store.StatusCancelled: "Anulowany",
}

var funcs = template.FuncMap{
	"money":  core.FormatMoney,
	"amount": core.FormatAmountInput,
	"dt":     core.HumanDT,
	"date":   core.HumanDate,
	"rate":   core.FormatRate,
	"status": func(s string) string { return statusLabels[s] },
	"billing": func(b string) string {
		if b == core.BillingHour {
			return "godzinowe"
		}
		return "dobowe"
	},
	"units": func(billing string, n int64) string { return core.UnitLabel(billing, n) },
	"deref": func(p *int64) any {
		if p == nil {
			return ""
		}
		return *p
	},
	"nl2br": func(s string) template.HTML {
		return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(s), "\n", "<br>"))
	},
	"kinds": func() []string { return store.VehicleKinds },
	"active": func(path, prefix string) bool {
		if prefix == "/" {
			return path == "/"
		}
		return strings.HasPrefix(path, prefix)
	},
	"today": func() string { return core.Today().Format(core.DateLayout) },
	"sub":   func(a, b int64) int64 { return a - b },
	"km": func(n int64) string {
		return strings.TrimSuffix(core.FormatAmount(n*100), ",00")
	},
	"kmp": func(p *int64) string {
		if p == nil {
			return ""
		}
		return strings.TrimSuffix(core.FormatAmount(*p*100), ",00") + " km"
	},
	"thousands": func(n int64) string { return strings.TrimSuffix(core.FormatAmount(n*100), ",00") },
}
