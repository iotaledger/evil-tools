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

const (
	maxBigWalletsCreatedAtOnce = 10
	checkFundsLeftInterval     = time.Second * 20
)

var log = logger.New("customSpam")

func requestFaucetFunds(params *CustomSpamParams, w *evilwallet.EvilWallet) <-chan bool {
	if params.SpamType == spammer.TypeBlock {
		return nil
	}
	var numOfBigWallets = 2
	fmt.Println("HERE, duration: ", params.Duration, "rate: ", params.Rate, "timeUnit: ", params.TimeUnit)
	if params.Duration >= 0 {
		numOfBigWallets = spammer.BigWalletsNeeded(params.Rate, params.TimeUnit, params.Duration)
		if numOfBigWallets > maxBigWalletsCreatedAtOnce {
			numOfBigWallets = maxBigWalletsCreatedAtOnce
			log.Warnf("Reached maximum number of big wallets created at once: %d, use infinite spam instead", maxBigWalletsCreatedAtOnce)
		}
	}
	fmt.Println("HERE, numOfBigWallets: ", numOfBigWallets)
	success := w.RequestFreshBigFaucetWallets(numOfBigWallets)
	if !success {
		log.Errorf("Failed to request faucet wallet")
		return nil
	}
	if params.Duration >= 0 {
		unspentOutputsLeft := w.UnspentOutputsLeft(evilwallet.Fresh)
		log.Debugf("Prepared %d unspent outputs for spamming.", unspentOutputsLeft)

		return nil
	}
	var requestingChan = make(<-chan bool)
	log.Debugf("Start requesting faucet funds infinitely...")
	go requestInfinitely(w, requestingChan)

	return requestingChan
}

func requestInfinitely(w *evilwallet.EvilWallet, done <-chan bool) {
	for {
		select {
		case <-done:
			log.Debug("Shutdown signal. Stopping requesting faucet funds for spam: %d", 0)

			return

		case <-time.After(checkFundsLeftInterval):
			outputsLeft := w.UnspentOutputsLeft(evilwallet.Fresh)
			// less than one big wallet left
			if outputsLeft < evilwallet.FaucetRequestSplitNumber*evilwallet.FaucetRequestSplitNumber {
				log.Debugf("Requesting new faucet funds, outputs left: %d", outputsLeft)
				err := w.RequestFreshBigFaucetWallet()
				if err != nil {
					log.Errorf("Failed to request faucet wallet: %s, stopping next requests...", err)

					return
				}

				log.Debugf("Requesting finished, currently available: %d unspent outputs for spamming.", w.UnspentOutputsLeft(evilwallet.Fresh))
			}
		}
	}
}

func CustomSpam(params *CustomSpamParams, accWallet *accountwallet.AccountWallet) {
	w := evilwallet.NewEvilWallet(evilwallet.WithClients(params.ClientURLs...), evilwallet.WithAccountsWallet(accWallet))
	wg := sync.WaitGroup{}

	log.Infof("Start spamming with rate: %d, time unit: %s, and spamming type: %s.", params.Rate, params.TimeUnit.String(), params.SpamType)

	var duration time.Duration = -1
	if params.Duration >= 0 {
		duration = params.Duration
	}

	// TODO here we can shutdown requesting when we will have evil-tools running in the background.
	_ = requestFaucetFunds(params, w)

	sType := params.SpamType

	switch sType {
	case spammer.TypeBlock:
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := SpamBlocks(w, params.Rate, params.TimeUnit, duration, params.EnableRateSetter, params.AccountAlias)
			if s == nil {
				return
			}
			s.Spam()
		}()
	case spammer.TypeBlowball:
		wg.Add(1)
		go func() {
			defer wg.Done()

			s := SpamBlowball(w, params.Rate, params.TimeUnit, duration, params.BlowballSize, params.EnableRateSetter, params.AccountAlias)
			if s == nil {
				return
			}
			s.Spam()
		}()
	case spammer.TypeTx:
		wg.Add(1)
		go func() {
			defer wg.Done()
			SpamTransaction(w, params.Rate, params.TimeUnit, duration, params.DeepSpam, params.EnableRateSetter, params.AccountAlias)
		}()
	case spammer.TypeDs:
		wg.Add(1)
		go func() {
			defer wg.Done()
			SpamDoubleSpends(w, params.Rate, params.NSpend, params.TimeUnit, duration, params.DelayBetweenConflicts, params.DeepSpam, params.EnableRateSetter, params.AccountAlias)
		}()
	case spammer.TypeCustom:
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := SpamNestedConflicts(w, params.Rate, params.TimeUnit, duration, params.Scenario, params.DeepSpam, false, params.EnableRateSetter, params.AccountAlias)
			if s == nil {
				return
			}
			s.Spam()
		}()
	case spammer.TypeAccounts:
		wg.Add(1)
		go func() {
			defer wg.Done()

			s := SpamAccounts(w, params.Rate, params.TimeUnit, duration, params.EnableRateSetter, params.AccountAlias)
			if s == nil {
				return
			}
			s.Spam()
		}()

	default:
		log.Warn("Spamming type not recognized. Try one of following: tx, ds, blk, custom, commitments")
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
