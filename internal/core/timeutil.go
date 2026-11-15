package core

import (
	"strings"
	"time"
)

// All dates are local wall-clock times stored as text. They are handled as
// UTC internally so that a "doba" is always 24 hours, also across DST changes.

const (
	DTLayout   = "2006-01-02T15:04" // HTML datetime-local, stored in DB
	DateLayout = "2006-01-02"       // HTML date, stored in DB
)

// ParseDT parses a datetime-local value.
func ParseDT(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) > 16 {
		s = s[:16]
	}
	return time.ParseInLocation(DTLayout, s, time.UTC)
}

// ParseDate parses a date value.
func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(DateLayout, strings.TrimSpace(s), time.UTC)
}

// Now returns the current local wall-clock time (minute precision).
func Now() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), n.Hour(), n.Minute(), 0, 0, time.UTC)
}

// Today returns today's local date at midnight.
func Today() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func FormatDT(t time.Time) string { return t.Format(DTLayout) }

// HumanDT turns a stored datetime into "15.11.2026 14:30".
func HumanDT(s string) string {
	t, err := ParseDT(s)
	if err != nil {
		return s
	}
	return t.Format("02.01.2006 15:04")
}

// HumanDate turns a stored date (or datetime) into "15.11.2026".
func HumanDate(s string) string {
	if len(s) > 10 {
		s = s[:10]
	}
	t, err := ParseDate(s)
	if err != nil {
		return s
	}
	return t.Format("02.01.2006")
}
