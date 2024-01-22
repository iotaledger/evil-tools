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

	fmt.Printf("%-10s \t%-33s\n\n", "Alias", "AccountID")
	for alias, wallet := range a.wallets {
		fmt.Printf("%-10s \t", alias)
		if wallet.accountData != nil {
			fmt.Printf("%-33s ", wallet.accountData.Account.Address().AccountID().ToHex())
		}
		fmt.Printf("\n")
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
