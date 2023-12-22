package spammer

import (
	"time"

	"github.com/iotaledger/evil-tools/pkg/evilwallet"
)

// BigWalletsNeeded calculates how many big wallets needs to be prepared for a spam based on provided spam details.
func BigWalletsNeeded(rate int, duration time.Duration) int {
	bigWalletSize := evilwallet.FaucetRequestSplitNumber * evilwallet.FaucetRequestSplitNumber
	outputsNeeded := rate * int(duration/time.Second)
	walletsNeeded := outputsNeeded/bigWalletSize + 1

	return walletsNeeded
}
