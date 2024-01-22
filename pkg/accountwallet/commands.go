package accountwallet

import (
	"context"
	"fmt"

	iotago "github.com/iotaledger/iota.go/v4"
)

// CreateAccount creates an implicit account and immediately transition it to a regular account.
func (a *AccountWallets) CreateAccount(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	if params.Implicit {
		return a.createImplicitAccount(ctx, params)
	}

	return a.createAccountWithFaucet(ctx, params)
}

func (a *AccountWallets) DestroyAccount(ctx context.Context, params *DestroyAccountParams) error {
	return a.destroyAccount(ctx, params.AccountAlias)
}

func (a *AccountWallets) ListAccount() error {
	a.walletsMutex.RLock()
	defer a.walletsMutex.RUnlock()

	hrp := a.API.ProtocolParameters().Bech32HRP()

	for alias, wallet := range a.wallets {
		if wallet.accountData == nil {
			continue
		}

		fmt.Printf("----------\n")
		fmt.Printf("%-10s %-33s\n", "Alias", alias)
		fmt.Printf("%-10s %-33s\n", "AccountID", wallet.accountData.Account.Address().AccountID().ToHex())
		fmt.Printf("%-10s %-33s\n", "Bech32", wallet.accountData.Account.Address().Bech32(hrp))
	}

	return nil
}

func (a *AccountWallets) AllotToAccount(_ *AllotAccountParams) error {
	return nil
}

func (a *AccountWallets) DelegateToAccount(ctx context.Context, params *DelegateAccountParams) error {
	return a.delegateToAccount(ctx, params)
}

func (a *AccountWallets) Rewards(ctx context.Context, params *RewardsAccountParams) error {
	return a.rewards(ctx, params)
}
