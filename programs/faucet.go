package programs

import (
	"context"
	"time"

	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/pkg/evilwallet"
	"github.com/iotaledger/evil-tools/pkg/spammer"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
)

const (
	MaxPendingRequestsRunning = 4
	FaucetFundsAwaitTimeout   = 3 * time.Minute
	SelectCheckInterval       = 5 * time.Second
)

func faucetFundsNeededForSpamType(spamType string) bool {
	switch spamType {
	case spammer.TypeBlock, spammer.TypeBlowball:
		return false
	}

	return true
}

func RequestFaucetFunds(ctx context.Context, logger log.Logger, paramsSpammer *spammer.ParametersSpammer, w *evilwallet.EvilWallet, totalWalletsNeeded int, minFundsDeposit int, faucetSplitNumber int) {
	if !faucetFundsNeededForSpamType(paramsSpammer.Type) {
		return
	}

	walletsReady := atomic.NewInt32(0)
	running := atomic.NewInt32(0)

	allRequested := func() bool {
		if paramsSpammer.Duration == spammer.InfiniteDuration {
			return false
		}

		return walletsReady.Load() >= int32(totalWalletsNeeded)
	}

	canSubmit := func() bool {
		return running.Load() < MaxPendingRequestsRunning && w.UnspentOutputsLeft(evilwallet.Fresh) <= 2*minFundsDeposit
	}

	if paramsSpammer.Duration == spammer.InfiniteDuration {
		logger.LogInfof("Wallet size: %d, Infinitely requesting faucet funds in the background...", faucetSplitNumber*faucetSplitNumber)
	} else {
		logger.LogInfof("Requesting faucet funds in the background, total wallets needed %d of size %d", totalWalletsNeeded, faucetSplitNumber*faucetSplitNumber)
	}
	wp := workerpool.New("Funds Requesting", workerpool.WithWorkerCount(2),
		workerpool.WithCancelPendingTasksOnShutdown(true))
	wp.Start()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(SelectCheckInterval):
			if allRequested() {
				wp.Shutdown()

				return
			}

			if !canSubmit() {
				continue
			}

			wp.Submit(func() {
				running.Inc()
				logger.LogInfof("Requesting faucet funds, preparing wallets, ready %d, in progress %d", walletsReady.Load(), running.Load())
				err := w.RequestFreshBigFaucetWallet(ctx)
				running.Dec()
				if err != nil {
					logger.LogErrorf("Failed to request wallet from faucet: %s", err)

					return
				}

				walletsReady.Inc()
			})
		}
	}
}
