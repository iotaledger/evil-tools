package models

type Config struct {
	WebAPI               []string `json:"webAPI"` //nolint:tagliatelle
	FaucetURL            string   `json:"faucetUrl"`
	Rate                 int      `json:"rate"`
	Duration             string   `json:"duration"`
	TimeUnit             string   `json:"timeUnit"`
	Deep                 bool     `json:"deepEnabled"`
	Reuse                bool     `json:"reuseEnabled,omitempty"`
	Scenario             string   `json:"scenario"`
	AutoRequesting       bool     `json:"autoRequestingEnabled,omitempty"`
	AutoRequestingAmount string   `json:"autoRequestingAmount,omitempty"`
	UseRateSetter        bool     `json:"useRateSetter,omitempty"`
}
