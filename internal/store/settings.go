package store

import (
	"strconv"
)

// Settings are the company data and calculation options.
type Settings struct {
	CompanyName      string
	CompanyAddress   string
	CompanyNIP       string
	CompanyPhone     string
	CompanyEmail     string
	BankAccount      string
	ContractCity     string
	ContractTerms    string
	VATRate          int // basis points
	ToleranceMinutes int // grace for the last started day
	BufferMinutes    int // free time required between rentals of a vehicle
	ReminderDays     int // warn about inspections / insurance this many days ahead
	NumberPrefix     string
}

const defaultTerms = `1. Najemca zobowiązuje się używać pojazdu zgodnie z jego przeznaczeniem i zwrócić go w stanie niepogorszonym, z wyjątkiem normalnego zużycia.
2. Najemca ponosi odpowiedzialność za szkody powstałe z jego winy w okresie najmu oraz za mandaty i opłaty związane z użytkowaniem pojazdu.
3. Zabronione jest podnajmowanie pojazdu oraz udostępnianie go osobom niewskazanym w umowie.
4. Przekroczenie limitu kilometrów jest dodatkowo płatne według stawki określonej w umowie.
5. Kaucja podlega zwrotowi po zwrocie pojazdu, po potrąceniu ewentualnych należności Wynajmującego.
6. Pojazd należy zwrócić z takim samym poziomem paliwa jak przy wydaniu.`

func defaultSettings() Settings {
	return Settings{
		CompanyName:      "Moja Wypożyczalnia",
		ContractTerms:    defaultTerms,
		VATRate:          2300,
		ToleranceMinutes: 60,
		BufferMinutes:    0,
		ReminderDays:     30,
		NumberPrefix:     "UM",
	}
}

func (s *Store) Settings() (Settings, error) {
	st := defaultSettings()
	rows, err := s.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
		}
		n, _ := strconv.Atoi(v)
		switch k {
		case "company_name":
			st.CompanyName = v
		case "company_address":
			st.CompanyAddress = v
		case "company_nip":
			st.CompanyNIP = v
		case "company_phone":
			st.CompanyPhone = v
		case "company_email":
			st.CompanyEmail = v
		case "bank_account":
			st.BankAccount = v
		case "contract_city":
			st.ContractCity = v
		case "contract_terms":
			st.ContractTerms = v
		case "vat_rate":
			st.VATRate = n
		case "tolerance_minutes":
			st.ToleranceMinutes = n
		case "buffer_minutes":
			st.BufferMinutes = n
		case "reminder_days":
			st.ReminderDays = n
		case "number_prefix":
			st.NumberPrefix = v
		}
	}
	return st, rows.Err()
}

func (s *Store) SaveSettings(st Settings) error {
	vals := map[string]string{
		"company_name":      st.CompanyName,
		"company_address":   st.CompanyAddress,
		"company_nip":       st.CompanyNIP,
		"company_phone":     st.CompanyPhone,
		"company_email":     st.CompanyEmail,
		"bank_account":      st.BankAccount,
		"contract_city":     st.ContractCity,
		"contract_terms":    st.ContractTerms,
		"vat_rate":          strconv.Itoa(st.VATRate),
		"tolerance_minutes": strconv.Itoa(st.ToleranceMinutes),
		"buffer_minutes":    strconv.Itoa(st.BufferMinutes),
		"reminder_days":     strconv.Itoa(st.ReminderDays),
		"number_prefix":     st.NumberPrefix,
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range vals {
		if _, err := tx.Exec("INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}
