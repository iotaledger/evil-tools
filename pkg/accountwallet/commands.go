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
	a.accountAliasesMutex.RLock()
	defer a.accountAliasesMutex.RUnlock()

	hrp := a.API.ProtocolParameters().Bech32HRP()

	for _, accData := range a.accountsAliases {
		fmt.Printf("----------\n")
		fmt.Printf("%-10s %-33s\n", "Alias", accData.Alias)
		fmt.Printf("%-10s %-33s\n", "AccountID", accData.Account.Address().AccountID().ToHex())
		fmt.Printf("%-10s %-33s\n", "Bech32", accData.Account.Address().Bech32(hrp))
	}

	return nil
}

func (a *AccountWallet) AllotToAccount(_ *AllotAccountParams) error {
	return nil
}
