package programs

import (
	"time"

	"github.com/iotaledger/evil-tools/evilwallet"
	"github.com/iotaledger/evil-tools/models"
)

type CustomSpamParams struct {
	ClientURLs            []string
	FaucetURL             string
	SpamType              string
	Rate                  int
	Duration              time.Duration
	TimeUnit              time.Duration
	DelayBetweenConflicts time.Duration
	NSpend                int
	Scenario              evilwallet.EvilBatch
	ScenarioName          string
	DeepSpam              bool
	EnableRateSetter      bool
	AccountAlias          string
	BlowballSize          int
}

func ConfigFromCustomSpamParams(params *CustomSpamParams) *models.Config {
	return &models.Config{
		WebAPI:   params.ClientURLs,
		FaucetURL: "http://localhost:8088",
		Rate:     params.Rate,
		Duration: params.Duration.String(),
		TimeUnit: params.TimeUnit.String(),
		Deep:     params.DeepSpam,
		Reuse:    false,
		Scenario: params.ScenarioName,
	}
}
