package programs

import (
	"time"

	"github.com/iotaledger/evil-tools/evilwallet"
	"github.com/iotaledger/evil-tools/models"
)

type CustomSpamParams struct {
	ClientURLs            []string
	SpamTypes             []string
	Rates                 []int
	Durations             []time.Duration
	BlkToBeSent           []int
	TimeUnit              time.Duration
	DelayBetweenConflicts time.Duration
	NSpend                int
	Scenario              evilwallet.EvilBatch
	ScenarioName          string
	DeepSpam              bool
	EnableRateSetter      bool
	AccountAlias          string
}

func ConfigFromCustomSpamParams(params *CustomSpamParams) *models.Config {
	return &models.Config{
		WebAPI:   params.ClientURLs,
		Rate:     params.Rates[0],
		Duration: params.Durations[0].String(),
		TimeUnit: params.TimeUnit.String(),
		Deep:     params.DeepSpam,
		Reuse:    false,
		Scenario: params.ScenarioName,
	}
}
