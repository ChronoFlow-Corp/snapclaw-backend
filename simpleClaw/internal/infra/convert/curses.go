package convert

type exchangeRate struct {
	Result             string             `json:"result"`
	Provider           string             `json:"provider"`
	Documentation      string             `json:"documentation"`
	TermsOfUse         string             `json:"terms_of_use"`
	TimeLastUpdateUnix int64              `json:"time_last_update_unix"`
	TimeLastUpdateUTC  string             `json:"time_last_update_utc"`
	TimeNextUpdateUnix int64              `json:"time_next_update_unix"`
	TimeNextUpdateUTC  string             `json:"time_next_update_utc"`
	TimeEOLUnix        int64              `json:"time_eol_unix"`
	BaseCode           string             `json:"base_code"`
	Rates              map[string]float64 `json:"rates"`
}
