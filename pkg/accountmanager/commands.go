package accountmanager

import (
	"context"
	"fmt"

	iotago "github.com/iotaledger/iota.go/v4"
)

// CreateAccount creates an implicit account and immediately transition it to a regular account.
func (m *Manager) CreateAccount(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	if params.Implicit {
		return m.createImplicitAccount(ctx, params)
	}

	return m.createAccountWithFaucet(ctx, params)
}

func (m *Manager) DestroyAccount(ctx context.Context, params *DestroyAccountParams) error {
	return m.destroyAccount(ctx, params.AccountAlias)
}

func (m *Manager) ListAccount() error {
	m.RLock()
	defer m.RUnlock()

	hrp := m.API.ProtocolParameters().Bech32HRP()

	for alias, accountData := range m.accounts {
		fmt.Printf("----------\n")
		fmt.Printf("%-10s %-33s\n", "Alias", alias)
		fmt.Printf("%-10s %-33s\n", "AccountID", accountData.Account.Address().AccountID().ToHex())
		fmt.Printf("%-10s %-33s\n", "Bech32", accountData.Account.Address().Bech32(hrp))
	}

	return nil
}

func (m *Manager) AllotToAccount(_ *AllotAccountParams) error {
	return nil
}

func (m *Manager) DelegateToAccount(ctx context.Context, params *DelegateAccountParams) error {
	return m.delegateToAccount(ctx, params)
}

func (m *Manager) Rewards(ctx context.Context, params *RewardsAccountParams) error {
	return m.rewards(ctx, params)
}

func (m *Manager) Delegators() map[iotago.OutputID]string {
	delegatedOutputs := make(map[iotago.OutputID]string)
	for alias, wallet := range m.wallets {
		for _, out := range wallet.delegationOutputs {
			delegatedOutputs[out.OutputID] = alias
		}
	}

	return delegatedOutputs
}
