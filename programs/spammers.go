package programs

import (
	"fmt"
	"sync"
	"time"

	"github.com/iotaledger/evil-tools/accountwallet"
	"github.com/iotaledger/evil-tools/evilwallet"
	"github.com/iotaledger/evil-tools/logger"
	"github.com/iotaledger/evil-tools/spammer"
)

var log = logger.New("customSpam")

func CustomSpam(params *CustomSpamParams, accWallet *accountwallet.AccountWallet) {
	w := evilwallet.NewEvilWallet(evilwallet.WithClients(params.ClientURLs...), evilwallet.WithAccountsWallet(accWallet))
	wg := sync.WaitGroup{}

	for i, sType := range params.SpamTypes {
		log.Infof("Start spamming with rate: %d, time unit: %s, and spamming type: %s.", params.Rates[i], params.TimeUnit.String(), sType)

		var duration time.Duration = -1
		if len(params.Durations) > i {
			duration = params.Durations[i]
		}
		// faucet funds preparation
		numOfBigWallets := spammer.BigWalletsNeeded(params.Rates[i], params.TimeUnit, duration)
		fmt.Println("numOfBigWallets: ", numOfBigWallets)
		success := w.RequestFreshBigFaucetWallets(numOfBigWallets)
		if !success {
			log.Errorf("Failed to request faucet wallet")
		}
		if sType != spammer.TypeBlock && sType != spammer.TypeBlowball {
			numOfBigWallets := spammer.BigWalletsNeeded(params.Rates[i], params.TimeUnit, params.Durations[i])
			fmt.Println("numOfBigWallets: ", numOfBigWallets)
			success := w.RequestFreshBigFaucetWallets(numOfBigWallets)
			if !success {
				log.Errorf("Failed to request faucet wallet")
				return
			}

			unspentOutputsLeft := w.UnspentOutputsLeft(evilwallet.Fresh)
			log.Debugf("Prepared %d unspent outputs for spamming.", unspentOutputsLeft)
		}

		switch sType {
		case spammer.TypeBlock:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				s := SpamBlocks(w, params.Rates[i], params.TimeUnit, duration, params.EnableRateSetter, params.AccountAlias)
				if s == nil {
					return
				}
				s.Spam()
			}(i)
		case spammer.TypeBlowball:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				s := SpamBlowball(w, params.Rates[i], params.TimeUnit, params.Durations[i], params.BlowballSize, params.EnableRateSetter, params.AccountAlias)
				if s == nil {
					return
				}
				s.Spam()
			}(i)
		case spammer.TypeTx:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				SpamTransaction(w, params.Rates[i], params.TimeUnit, duration, params.DeepSpam, params.EnableRateSetter, params.AccountAlias)
			}(i)
		case spammer.TypeDs:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				SpamDoubleSpends(w, params.Rates[i], params.NSpend, params.TimeUnit, duration, params.DelayBetweenConflicts, params.DeepSpam, params.EnableRateSetter, params.AccountAlias)
			}(i)
		case spammer.TypeCustom:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				s := SpamNestedConflicts(w, params.Rates[i], params.TimeUnit, duration, params.Scenario, params.DeepSpam, false, params.EnableRateSetter, params.AccountAlias)
				if s == nil {
					return
				}
				s.Spam()
			}(i)
		case spammer.TypeAccounts:
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				s := SpamAccounts(w, params.Rates[i], params.TimeUnit, duration, params.EnableRateSetter, params.AccountAlias)
				if s == nil {
					return
				}
				s.Spam()
			}(i)

		default:
			log.Warn("Spamming type not recognized. Try one of following: tx, ds, blk, custom, commitments")
		}
	}

	wg.Wait()
	log.Info("Basic spamming finished!")
}

func SpamTransaction(w *evilwallet.EvilWallet, rate int, timeUnit, duration time.Duration, deepSpam, enableRateSetter bool, accountAlias string) {
	if w.NumOfClient() < 1 {
		log.Infof("Warning: At least one client is needed to spam.")
	}

	scenarioOptions := []evilwallet.ScenarioOption{
		evilwallet.WithScenarioCustomConflicts(evilwallet.SingleTransactionBatch()),
	}
	if deepSpam {
		outWallet := evilwallet.NewWallet(evilwallet.Reuse)
		scenarioOptions = append(scenarioOptions,
			evilwallet.WithScenarioDeepSpamEnabled(),
			evilwallet.WithScenarioReuseOutputWallet(outWallet),
			evilwallet.WithScenarioInputWalletForDeepSpam(outWallet),
		)
	}
	scenarioTx := evilwallet.NewEvilScenario(scenarioOptions...)

	options := []spammer.Options{
		spammer.WithSpamRate(rate, timeUnit),
		spammer.WithSpamDuration(duration),
		spammer.WithRateSetter(enableRateSetter),
		spammer.WithEvilWallet(w),
		spammer.WithEvilScenario(scenarioTx),
		spammer.WithAccountAlias(accountAlias),
	}

	s := spammer.NewSpammer(options...)
	s.Spam()
}

func SpamDoubleSpends(w *evilwallet.EvilWallet, rate, nSpent int, timeUnit, duration, delayBetweenConflicts time.Duration, deepSpam, enableRateSetter bool, accountAlias string) {
	log.Debugf("Setting up double spend spammer with rate: %d, time unit: %s, and duration: %s.", rate, timeUnit.String(), duration.String())
	if w.NumOfClient() < 2 {
		log.Infof("Warning: At least two client are needed to spam, and %d was provided", w.NumOfClient())
	}

	scenarioOptions := []evilwallet.ScenarioOption{
		evilwallet.WithScenarioCustomConflicts(evilwallet.NSpendBatch(nSpent)),
	}
	if deepSpam {
		outWallet := evilwallet.NewWallet(evilwallet.Reuse)
		scenarioOptions = append(scenarioOptions,
			evilwallet.WithScenarioDeepSpamEnabled(),
			evilwallet.WithScenarioReuseOutputWallet(outWallet),
			evilwallet.WithScenarioInputWalletForDeepSpam(outWallet),
		)
	}
	scenarioDs := evilwallet.NewEvilScenario(scenarioOptions...)
	options := []spammer.Options{
		spammer.WithSpamRate(rate, timeUnit),
		spammer.WithSpamDuration(duration),
		spammer.WithEvilWallet(w),
		spammer.WithRateSetter(enableRateSetter),
		spammer.WithTimeDelayForDoubleSpend(delayBetweenConflicts),
		spammer.WithEvilScenario(scenarioDs),
		spammer.WithAccountAlias(accountAlias),
	}

	s := spammer.NewSpammer(options...)
	s.Spam()
}

func SpamNestedConflicts(w *evilwallet.EvilWallet, rate int, timeUnit, duration time.Duration, conflictBatch evilwallet.EvilBatch, deepSpam, reuseOutputs, enableRateSetter bool, accountAlias string) *spammer.Spammer {
	scenarioOptions := []evilwallet.ScenarioOption{
		evilwallet.WithScenarioCustomConflicts(conflictBatch),
	}
	if deepSpam {
		outWallet := evilwallet.NewWallet(evilwallet.Reuse)
		scenarioOptions = append(scenarioOptions,
			evilwallet.WithScenarioDeepSpamEnabled(),
			evilwallet.WithScenarioReuseOutputWallet(outWallet),
			evilwallet.WithScenarioInputWalletForDeepSpam(outWallet),
		)
	} else if reuseOutputs {
		outWallet := evilwallet.NewWallet(evilwallet.Reuse)
		scenarioOptions = append(scenarioOptions, evilwallet.WithScenarioReuseOutputWallet(outWallet))
	}
	scenario := evilwallet.NewEvilScenario(scenarioOptions...)
	if scenario.NumOfClientsNeeded > w.NumOfClient() {
		log.Infof("Warning: At least %d client are needed to spam, and %d was provided", scenario.NumOfClientsNeeded, w.NumOfClient())
	}

	options := []spammer.Options{
		spammer.WithSpamRate(rate, timeUnit),
		spammer.WithSpamDuration(duration),
		spammer.WithEvilWallet(w),
		spammer.WithRateSetter(enableRateSetter),
		spammer.WithEvilScenario(scenario),
		spammer.WithAccountAlias(accountAlias),
	}

	return spammer.NewSpammer(options...)
}

func SpamBlocks(w *evilwallet.EvilWallet, rate int, timeUnit, duration time.Duration, enableRateSetter bool, accountAlias string) *spammer.Spammer {
	if w.NumOfClient() < 1 {
		log.Infof("Warning: At least one client is needed to spam.")
	}

	options := []spammer.Options{
		spammer.WithSpamRate(rate, timeUnit),
		spammer.WithSpamDuration(duration),
		spammer.WithRateSetter(enableRateSetter),
		spammer.WithEvilWallet(w),
		spammer.WithSpammingFunc(spammer.DataSpammingFunction),
		spammer.WithAccountAlias(accountAlias),
	}

	return spammer.NewSpammer(options...)
}

func SpamAccounts(w *evilwallet.EvilWallet, rate int, timeUnit, duration time.Duration, enableRateSetter bool, accountAlias string) *spammer.Spammer {
	if w.NumOfClient() < 1 {
		log.Infof("Warning: At least one client is needed to spam.")
	}
	scenarioOptions := []evilwallet.ScenarioOption{
		evilwallet.WithScenarioCustomConflicts(evilwallet.SingleTransactionBatch()),
		evilwallet.WithCreateAccounts(),
	}

	scenarioAccount := evilwallet.NewEvilScenario(scenarioOptions...)

	options := []spammer.Options{
		spammer.WithSpamRate(rate, timeUnit),
		spammer.WithSpamDuration(duration),
		spammer.WithRateSetter(enableRateSetter),
		spammer.WithEvilWallet(w),
		spammer.WithSpammingFunc(spammer.AccountSpammingFunction),
		spammer.WithEvilScenario(scenarioAccount),
		spammer.WithAccountAlias(accountAlias),
	}

	return spammer.NewSpammer(options...)
}

func SpamBlowball(w *evilwallet.EvilWallet, rate int, timeUnit, duration time.Duration, blowballSize int, enableRateSetter bool, accountAlias string) *spammer.Spammer {
	if w.NumOfClient() < 1 {
		log.Infof("Warning: At least one client is needed to spam.")
	}

	// blowball spammer needs at least 40 seconds to finish
	if duration < 40*time.Second {
		duration = 40 * time.Second
	}

	options := []spammer.Options{
		spammer.WithSpamRate(rate, timeUnit),
		spammer.WithSpamDuration(duration),
		spammer.WithBlowballSize(blowballSize),
		spammer.WithRateSetter(enableRateSetter),
		spammer.WithEvilWallet(w),
		spammer.WithSpammingFunc(spammer.BlowballSpammingFunction),
		spammer.WithAccountAlias(accountAlias),
	}

	return spammer.NewSpammer(options...)
}
