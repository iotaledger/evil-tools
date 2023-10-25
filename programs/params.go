package programs

import (
	"time"

	"github.com/iotaledger/evil-tools/evilwallet"
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
	DeepSpam              bool
	EnableRateSetter      bool
	AccountAlias          string
}
