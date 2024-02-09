package spammer

import (
	"time"

	"github.com/iotaledger/evil-tools/pkg/evilwallet"
	"github.com/iotaledger/hive.go/ds/types"
)

// BigWalletsNeeded calculates how many big wallets needs to be prepared for a spam based on provided spam details.
func BigWalletsNeeded(rate int, duration time.Duration, spammingBatchSize int) int {
	bigWalletSize := evilwallet.FaucetRequestSplitNumber * evilwallet.FaucetRequestSplitNumber
	outputsNeeded := rate * int(duration/time.Second) * spammingBatchSize
	walletsNeeded := outputsNeeded/bigWalletSize + 1

	return walletsNeeded
}

func MinFaucetFundsDeposit(rate int, duration time.Duration, spammingBatchSize int) int {
	minSpamDurationFundsReady := MinSpamDurationFundsReady
	if duration != InfiniteDuration && duration < MinSpamDurationFundsReady {
		minSpamDurationFundsReady = duration
	}
	minFaucetFundsDeposit := int(minSpamDurationFundsReady/time.Second) * rate * spammingBatchSize

	return minFaucetFundsDeposit
}

func EvaluateNumOfBatchInputs(params *ParametersSpammer) int {
	scenario, ok := evilwallet.GetScenario(params.Type)
	if !ok {
		return 1
	}

	var numberOfFreshInputs int
	switch params.Type {
	case TypeDs:
		return params.NSpend
	case TypeBlock, TypeTx, TypeAccounts, TypeBlowball:
		return 1
	default:
		// gather all the outputs aliases
		outputs := make(map[string]types.Empty)
		for _, scenarioAlias := range scenario {
			for _, batch := range scenarioAlias {
				for _, output := range batch.Outputs {
					outputs[output] = types.Void
				}
			}
			outputs[scenarioAlias[0].Outputs[0]] = types.Void
		}

		for _, scenarioAlias := range scenario {
			for _, batch := range scenarioAlias {
				for _, input := range batch.Inputs {
					if _, ok = outputs[input]; !ok {
						// count all inputs that did not come from an output
						numberOfFreshInputs++
					}
				}
			}
		}
	}

	return numberOfFreshInputs
}
