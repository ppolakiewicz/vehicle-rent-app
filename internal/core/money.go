// Package core holds the business rules that do not need the database:
// money and time helpers and the rental price calculation.
package core

import (
	"errors"
	"strconv"
	"strings"
)

// Amounts are kept as int64 grosze (1/100 PLN) to avoid rounding errors.

// FormatMoney formats grosze as "1 234,56 zł".
func FormatMoney(g int64) string {
	return FormatAmount(g) + " zł"
}

// FormatAmount formats grosze as "1 234,56" (no currency).
func FormatAmount(g int64) string {
	neg := g < 0
	if neg {
		g = -g
	}
	zl := strconv.FormatInt(g/100, 10)
	var b strings.Builder
	for i, r := range zl {
		if i > 0 && (len(zl)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	s := b.String() + "," + pad2(g%100)
	if neg {
		s = "-" + s
	}
	return s
}

// FormatAmountInput formats grosze for a form field: "1234,56".
func FormatAmountInput(g int64) string {
	neg := g < 0
	if neg {
		g = -g
	}
	s := strconv.FormatInt(g/100, 10) + "," + pad2(g%100)
	if neg {
		s = "-" + s
	}
	return s
}

func pad2(v int64) string {
	if v < 10 {
		return "0" + strconv.FormatInt(v, 10)
	}
	return strconv.FormatInt(v, 10)
}

var ErrBadAmount = errors.New("nieprawidłowa kwota")

// ParseMoney parses "1234,56", "1 234.5", "100 zł" into grosze. Empty is 0.
func ParseMoney(s string) (int64, error) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "zł"))
	s = strings.NewReplacer(" ", "", " ", "", ",", ".").Replace(s)
	if s == "" {
		return 0, nil
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}
	if len(frac) > 2 || strings.Contains(frac, ".") {
		return 0, ErrBadAmount
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, ErrBadAmount
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, ErrBadAmount
	}
	v := w*100 + f
	if neg {
		v = -v
	}
	return v, nil
}

// VAT returns the VAT for a net amount; rate is in basis points (2300 = 23%).
// Rounds half away from zero to full grosze.
func VAT(net int64, rate int) int64 {
	v := net * int64(rate)
	if v >= 0 {
		return (v + 5000) / 10000
	}
	return -((-v + 5000) / 10000)
}

// FormatRate formats basis points as "23" or "8,5".
func FormatRate(bp int) string {
	s := strconv.Itoa(bp / 100)
	if r := bp % 100; r != 0 {
		f := strings.TrimRight(pad2(int64(r)), "0")
		s += "," + f
	}
	return s
}

// ParseRate parses "23" or "8,5" (percent) into basis points.
func ParseRate(s string) (int, error) {
	v, err := ParseMoney(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	if err != nil || v < 0 || v > 10000 {
		return 0, errors.New("nieprawidłowa stawka VAT")
	}
	return int(v), nil
}
