package accountwallet

import (
	"context"
	"fmt"

	iotago "github.com/iotaledger/iota.go/v4"
)

// CreateAccount creates an implicit account and immediately transition it to a regular account.
func (a *AccountWallet) CreateAccount(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	if params.Implicit {
		return a.createImplicitAccount(ctx, params)
	}

	return a.createAccountWithFaucet(ctx, params)
}

func (a *AccountWallet) DestroyAccount(ctx context.Context, params *DestroyAccountParams) error {
	return a.destroyAccount(ctx, params.AccountAlias)
}

func (a *AccountWallet) ListAccount() error {
	a.outputsMutex.RLock()
	defer a.outputsMutex.RUnlock()

	fmt.Printf("%-10s \t%-33s\n\n", "Alias", "AccountID")
	for _, accData := range a.accounts {
		fmt.Printf("%-10s \t", accData.Alias)
		fmt.Printf("%-33s ", accData.Account.Address().AccountID().ToHex())
		fmt.Printf("\n")
	}

	return nil
}

func (a *AccountWallet) AllotToAccount(_ *AllotAccountParams) error {
	return nil
}

func (a *AccountWallet) DelegateToAccount(ctx context.Context, params *DelegateAccountParams) error {
	return a.delegateToAccount(ctx, params)
}

func (a *AccountWallet) Rewards(ctx context.Context, params *RewardsAccountParams) error {
	return a.rewards(ctx, params)
}
