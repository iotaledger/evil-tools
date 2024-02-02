package programs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iotaledger/hive.go/log"

	"github.com/iotaledger/evil-tools/pkg/evilwallet"
	"github.com/iotaledger/evil-tools/pkg/spammer"
	"github.com/iotaledger/evil-tools/pkg/walletmanager"
)

func RunSpammer(ctx context.Context, logger log.Logger, nodeURLs []string, faucetURL string, paramsSpammer *spammer.ParametersSpammer, accManager *walletmanager.Manager) {
	fmt.Println("RunSpammer")
	w := evilwallet.NewEvilWallet(logger, evilwallet.WithClients(nodeURLs...), evilwallet.WithAccountsManager(accManager), evilwallet.WithFaucetClient(faucetURL))
	wg := sync.WaitGroup{}

	logger.LogInfof("Start spamming with rate: %d, spamming type: %s.", paramsSpammer.Rate, paramsSpammer.Type)

	// cancel is a context.CancelFunc that can be used to cancel the infinite requesting goroutine.
	_, err := requestFaucetFunds(ctx, logger, paramsSpammer, w)
	if err != nil {
		logger.LogWarnf("Failed to request faucet funds, stopping spammer: %v", err)
		return
	}

	switch paramsSpammer.Type {
	case spammer.TypeBlock:
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := SpamBlocks(logger, w, paramsSpammer)
			if s == nil {
				return
			}
			s.Spam(ctx)
		}()
	case spammer.TypeBlowball:
		wg.Add(1)
		go func() {
			defer wg.Done()

			s := SpamBlowball(logger, w, paramsSpammer)
			if s == nil {
				return
			}
			s.Spam(ctx)
		}()
	case spammer.TypeTx:
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := SpamTransaction(logger, w, paramsSpammer)
			s.Spam(ctx)
		}()
	case spammer.TypeDs:
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := SpamDoubleSpends(logger, w, paramsSpammer)
			s.Spam(ctx)
		}()
	case spammer.TypeAccounts:
		wg.Add(1)
		go func() {
			defer wg.Done()

			s := SpamAccounts(logger, w, paramsSpammer)
			if s == nil {
				return
			}
			s.Spam(ctx)
		}()

	default:
		if !evilwallet.IsScenarioAllowed(paramsSpammer.Type) {
			logger.LogFatal(fmt.Sprintf("Spamming type not recognized. Try one of following: tx, ds, blk, bb, accounts, %s", evilwallet.AllScenariosListed()))

			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			s := SpamNestedConflicts(logger, w, paramsSpammer)
			if s == nil {
				return
			}
			s.Spam(ctx)
		}()
	}

	wg.Wait()
	logger.LogInfo("Basic spamming finished!")
}

func SpamTransaction(logger log.Logger, w *evilwallet.EvilWallet, paramsSpammer *spammer.ParametersSpammer) *spammer.Spammer {
	if w.NumOfClient() < 1 {
		logger.LogInfo("Warning: At least one client is needed to spam.")
	}

	scenarioOptions := []evilwallet.ScenarioOption{
		evilwallet.WithScenarioCustomConflicts(evilwallet.SingleTransactionBatch()),
	}
	if paramsSpammer.DeepSpamEnabled {
		outWallet := w.NewWallet(evilwallet.Reuse)
		scenarioOptions = append(scenarioOptions,
			evilwallet.WithScenarioDeepSpamEnabled(),
			evilwallet.WithScenarioReuseOutputWallet(outWallet),
			evilwallet.WithScenarioInputWalletForDeepSpam(outWallet),
		)
	}
	scenario := evilwallet.NewEvilScenario(scenarioOptions...)

	return spammer.NewSpammer(logger,
		spammer.WithRate(paramsSpammer.Rate),
		spammer.WithSpamDuration(paramsSpammer.Duration),
		spammer.WithRateSetter(paramsSpammer.RateSetterEnabled),
		spammer.WithEvilWallet(w),
		spammer.WithEvilScenario(scenario),
		spammer.WithAccountAlias(paramsSpammer.Account),
	)
}

func SpamDoubleSpends(logger log.Logger, w *evilwallet.EvilWallet, paramsSpammer *spammer.ParametersSpammer) *spammer.Spammer {
	if w.NumOfClient() < 1 {
		logger.LogInfo("Warning: At least one client are needed to spam")
	}

	scenarioOptions := []evilwallet.ScenarioOption{
		evilwallet.WithScenarioCustomConflicts(evilwallet.NSpendBatch(paramsSpammer.NSpend)),
	}

	if paramsSpammer.DeepSpamEnabled {
		outWallet := w.NewWallet(evilwallet.Reuse)
		scenarioOptions = append(scenarioOptions,
			evilwallet.WithScenarioDeepSpamEnabled(),
			evilwallet.WithScenarioReuseOutputWallet(outWallet),
			evilwallet.WithScenarioInputWalletForDeepSpam(outWallet),
		)
	}

	scenario := evilwallet.NewEvilScenario(scenarioOptions...)

	return spammer.NewSpammer(logger,
		spammer.WithRate(paramsSpammer.Rate),
		spammer.WithSpamDuration(paramsSpammer.Duration),
		spammer.WithRateSetter(paramsSpammer.RateSetterEnabled),
		spammer.WithEvilWallet(w),
		spammer.WithEvilScenario(scenario),
		spammer.WithAccountAlias(paramsSpammer.Account),
	)
}

func SpamNestedConflicts(logger log.Logger, w *evilwallet.EvilWallet, paramsSpammer *spammer.ParametersSpammer) *spammer.Spammer {
	conflictBatch, ok := evilwallet.GetScenario(paramsSpammer.Type)
	if !ok {
		panic(fmt.Sprintf("Scenario not found: %s", paramsSpammer.Type))
	}

	scenarioOptions := []evilwallet.ScenarioOption{
		evilwallet.WithScenarioCustomConflicts(conflictBatch),
	}

	if paramsSpammer.DeepSpamEnabled {
		outWallet := w.NewWallet(evilwallet.Reuse)
		scenarioOptions = append(scenarioOptions,
			evilwallet.WithScenarioDeepSpamEnabled(),
			evilwallet.WithScenarioReuseOutputWallet(outWallet),
			evilwallet.WithScenarioInputWalletForDeepSpam(outWallet),
		)
	} else if paramsSpammer.ReuseEnabled {
		outWallet := evilwallet.NewWallet(evilwallet.Reuse)
		scenarioOptions = append(scenarioOptions, evilwallet.WithScenarioReuseOutputWallet(outWallet))
	}

	scenario := evilwallet.NewEvilScenario(scenarioOptions...)
	if scenario.NumOfClientsNeeded > w.NumOfClient() {
		logger.LogInfof("Warning: At least %d client are needed to spam, and %d was provided", scenario.NumOfClientsNeeded, w.NumOfClient())
	}

	return spammer.NewSpammer(logger,
		spammer.WithRate(paramsSpammer.Rate),
		spammer.WithSpamDuration(paramsSpammer.Duration),
		spammer.WithRateSetter(paramsSpammer.RateSetterEnabled),
		spammer.WithEvilWallet(w),
		spammer.WithEvilScenario(scenario),
		spammer.WithAccountAlias(paramsSpammer.Account),
	)
}

func SpamBlocks(logger log.Logger, w *evilwallet.EvilWallet, paramsSpammer *spammer.ParametersSpammer) *spammer.Spammer {
	if w.NumOfClient() < 1 {
		logger.LogInfo("Warning: At least one client is needed to spam.")
	}

	return spammer.NewSpammer(logger,
		spammer.WithRate(paramsSpammer.Rate),
		spammer.WithSpamDuration(paramsSpammer.Duration),
		spammer.WithRateSetter(paramsSpammer.RateSetterEnabled),
		spammer.WithEvilWallet(w),
		spammer.WithSpammingFunc(spammer.DataSpammingFunction),
		spammer.WithAccountAlias(paramsSpammer.Account),
	)
}

func SpamAccounts(logger log.Logger, w *evilwallet.EvilWallet, paramsSpammer *spammer.ParametersSpammer) *spammer.Spammer {
	if w.NumOfClient() < 1 {
		logger.LogInfo("Warning: At least one client is needed to spam.")
	}
	scenarioOptions := []evilwallet.ScenarioOption{
		evilwallet.WithScenarioCustomConflicts(evilwallet.SingleTransactionBatch()),
		evilwallet.WithCreateAccounts(),
	}
	scenarioAccount := evilwallet.NewEvilScenario(scenarioOptions...)

	return spammer.NewSpammer(logger,
		spammer.WithRate(paramsSpammer.Rate),
		spammer.WithSpamDuration(paramsSpammer.Duration),
		spammer.WithRateSetter(paramsSpammer.RateSetterEnabled),
		spammer.WithEvilWallet(w),
		spammer.WithSpammingFunc(spammer.AccountSpammingFunction),
		spammer.WithEvilScenario(scenarioAccount),
		spammer.WithAccountAlias(paramsSpammer.Account),
	)
}

func SpamBlowball(logger log.Logger, w *evilwallet.EvilWallet, paramsSpammer *spammer.ParametersSpammer) *spammer.Spammer {
	if w.NumOfClient() < 1 {
		logger.LogInfo("Warning: At least one client is needed to spam.")
	}

	// blowball spammer needs at least 40 seconds to finish
	if paramsSpammer.Duration < 40*time.Second {
		paramsSpammer.Duration = 40 * time.Second
	}

	return spammer.NewSpammer(logger,
		spammer.WithRate(paramsSpammer.Rate),
		spammer.WithSpamDuration(paramsSpammer.Duration),
		spammer.WithRateSetter(paramsSpammer.RateSetterEnabled),
		spammer.WithBlowballSize(paramsSpammer.BlowballSize),
		spammer.WithSpammingFunc(spammer.BlowballSpammingFunction),
		spammer.WithEvilWallet(w),
		spammer.WithAccountAlias(paramsSpammer.Account),
	)
}
