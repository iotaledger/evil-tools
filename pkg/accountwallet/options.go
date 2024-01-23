package accountwallet

import (
	"github.com/iotaledger/hive.go/runtime/options"
)

// WithClientURL sets the client bind address.
func WithClientURL(url string) options.Option[AccountWallets] {
	return func(a *AccountWallets) {
		a.optsClientBindAddress = url
	}
}

func WithFaucetURL(url string) options.Option[AccountWallets] {
	return func(a *AccountWallets) {
		a.optsFaucetURL = url
	}
}

func WithAccountStatesFile(fileName string) options.Option[AccountWallets] {
	return func(a *AccountWallets) {
		a.optsAccountStatesFile = fileName
	}
}

func WithFaucetAccountParams(params *GenesisAccountParams) options.Option[AccountWallets] {
	return func(a *AccountWallets) {
		a.optsGenesisAccountParams = params
	}
}
