package programs

import (
	"context"
	"time"

	"github.com/iotaledger/evil-tools/pkg/evilwallet"
	"github.com/iotaledger/evil-tools/pkg/spammer"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
)

func requestFaucetFunds(ctx context.Context, logger log.Logger, paramsSpammer *spammer.ParametersSpammer, w *evilwallet.EvilWallet) (context.CancelFunc, error) {
	if paramsSpammer.Type == spammer.TypeBlock {
		return nil, nil
	}

	var numOfBigWallets = evilwallet.BigFaucetWalletsAtOnce
	if paramsSpammer.Duration != spammer.InfiniteDuration {
		numNeeded := spammer.BigWalletsNeeded(paramsSpammer.Rate, paramsSpammer.Duration)
		if numNeeded > evilwallet.MaxBigWalletsCreatedAtOnce {
			numNeeded = evilwallet.MaxBigWalletsCreatedAtOnce
			logger.LogWarnf("Reached maximum number of big wallets created at once: %d, use infinite spam instead", evilwallet.MaxBigWalletsCreatedAtOnce)
		}
		numOfBigWallets = numNeeded
	}

	success := w.RequestFreshBigFaucetWallets(ctx, numOfBigWallets)
	if !success {
		logger.LogError("Failed to request faucet wallet")
		return nil, ierrors.Errorf("failed to request faucet wallet")
	}

	if paramsSpammer.Duration != spammer.InfiniteDuration {
		unspentOutputsLeft := w.UnspentOutputsLeft(evilwallet.Fresh)
		logger.LogDebugf("Prepared %d unspent outputs for spamming.", unspentOutputsLeft)

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
			if outputsLeft < evilwallet.BigFaucetWalletDeposit*evilwallet.FaucetRequestSplitNumber*evilwallet.FaucetRequestSplitNumber {
				logger.LogDebugf("Requesting new faucet funds, outputs left: %d", outputsLeft)
				success := w.RequestFreshBigFaucetWallets(ctx, evilwallet.BigFaucetWalletsAtOnce)
				if !success {
					logger.LogError("Failed to request faucet wallet, stopping next requests..., stopping spammer")

					return
				}

				logger.LogDebugf("Requesting finished, currently available: %d unspent outputs for spamming.", w.UnspentOutputsLeft(evilwallet.Fresh))
			}
		}
	}
}
