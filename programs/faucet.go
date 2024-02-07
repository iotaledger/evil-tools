package programs

import (
	"context"
	"fmt"
	"time"

	"github.com/iotaledger/evil-tools/pkg/evilwallet"
	"github.com/iotaledger/evil-tools/pkg/spammer"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
)

func requestFaucetFunds(ctx context.Context, logger log.Logger, paramsSpammer *spammer.ParametersSpammer, w *evilwallet.EvilWallet) (context.CancelFunc, error) {
	if paramsSpammer.Type == spammer.TypeBlock {
		return nil, nil
	}

	numOfInputs := spammer.EvaluateNumOfBatchInputs(paramsSpammer)
	fmt.Println("numOfInputs evaluated", numOfInputs)
	walletsNeeded := spammer.BigWalletsNeeded(paramsSpammer.Rate, paramsSpammer.Duration, numOfInputs)
	fmt.Println("walletsNeeded evaluated", walletsNeeded)
	success := w.RequestFreshBigFaucetWallets(ctx, lo.Min[int](walletsNeeded, evilwallet.BackgroundRequestingBigWalletsThreshold))
	if !success {
		logger.LogError("Failed to request faucet wallet funds. Spammer will try again.")
	}
	if success && spammer.InfiniteDuration != paramsSpammer.Duration && walletsNeeded < evilwallet.BackgroundRequestingBigWalletsThreshold {
		// no need for an additional funds

		return nil, nil
	}

	logger.LogDebug("Start requesting faucet funds infinitely...")
	infiniteCtx, cancel := context.WithCancel(ctx)
	go requestInfinitely(infiniteCtx, logger, w)

	return cancel, nil

}

func requestInfinitely(ctx context.Context, logger log.Logger, w *evilwallet.EvilWallet) {
	for {
		select {
		case <-ctx.Done():
			logger.LogDebugf("Shutdown signal. Stopping requesting faucet funds for spam: %d", 0)

			return

		case <-time.After(evilwallet.CheckFundsLeftInterval):
			outputsLeft := w.UnspentOutputsLeft(evilwallet.Fresh)
			// keep requesting over and over until we have at least deposit
			if outputsLeft < evilwallet.BigWalletDepositThreshold*evilwallet.FaucetRequestSplitNumber*evilwallet.FaucetRequestSplitNumber {
				logger.LogDebugf("Requesting new faucet funds, outputs left: %d", outputsLeft)
				// TODO forward here the ctx and send it to a separate go routine, use sematpore or some kind of wroker pool to limit number of ongoing requests
				success := w.RequestFreshBigFaucetWallets(ctx, evilwallet.BigFaucetWalletsAtOnce)
				if !success {
					logger.LogError("Failed to request faucet wallet, stopping next requests...")
				} else {
					logger.LogDebugf("Requesting finished, currently available: %d unspent outputs for spamming.", w.UnspentOutputsLeft(evilwallet.Fresh))
				}
			}
		}
	}
}
