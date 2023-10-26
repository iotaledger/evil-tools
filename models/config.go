package models

type Config struct {
	//nolint:tagliatelle
	WebAPI               []string `json:"webAPI"`
	Rate                 int      `json:"rate"`
	DurationStr          string   `json:"duration"`
	TimeUnitStr          string   `json:"timeUnit"`
	Deep                 bool     `json:"deepEnabled"`
	Reuse                bool     `json:"reuseEnabled"`
	Scenario             string   `json:"scenario"`
	AutoRequesting       bool     `json:"autoRequestingEnabled"`
	AutoRequestingAmount string   `json:"autoRequestingAmount"`
	UseRateSetter        bool     `json:"useRateSetter"`
}
