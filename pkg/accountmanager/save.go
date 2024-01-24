package accountmanager

import (
	"crypto/ed25519"
	"os"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/wallet"
)

func (m *Manager) toAccountState() *AccountsState {
	state := &AccountsState{
		accountState:       make(map[string]AccountState),
		wallets:            m.wallets,
		RequestTokenAmount: m.RequestTokenAmount,
		RequestManaAmount:  m.RequestManaAmount,
	}

	for alias, accountData := range m.accounts {
		state.accountState[alias] = AccountStateFromAccountData(accountData)
	}

	return state
}

func (m *Manager) fromAccountState(state *AccountsState) {
	m.Lock()
	defer m.Unlock()

	accountData := make(map[string]*models.AccountData)
	for alias, accState := range state.accountState {
		accountData[alias] = accState.ToAccountData()
	}

	m.accounts = accountData
	m.wallets = state.wallets
	m.RequestTokenAmount = state.RequestTokenAmount
	m.RequestManaAmount = state.RequestManaAmount
}

func (m *Manager) SaveStateToFile() error {
	m.Lock()
	defer m.Unlock()

	state := m.toAccountState()
	stateBytes, err := m.Client.LatestAPI().Encode(state)
	if err != nil {
		return ierrors.Wrap(err, "failed to encode account state")
	}

	//nolint:gosec // users should be able to read the file
	if err = os.WriteFile(m.optsAccountStatesFile, stateBytes, 0o644); err != nil {
		return ierrors.Wrap(err, "failed to write account states to file")
	}

	return nil
}

func (m *Manager) LoadStateFromFile() (loaded bool, err error) {
	m.Lock()
	defer m.Unlock()

	walletStateBytes, err := os.ReadFile(m.optsAccountStatesFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, ierrors.Wrap(err, "failed to read file")
		}

		return false, nil
	}

	var state AccountsState
	_, err = m.Client.LatestAPI().Decode(walletStateBytes, &state)
	if err != nil {
		return false, ierrors.Wrap(err, "failed to decode from file")
	}
	m.fromAccountState(&state)

	return true, nil
}

type AccountsState struct {
	accountState       map[string]AccountState   `serix:"accountState,lenPrefix=uint8"`
	wallets            map[string]*AccountWallet `serix:"wallets,lenPrefix=uint8"`
	RequestTokenAmount iotago.BaseToken          `serix:"RequestTokenAmount"`
	RequestManaAmount  iotago.Mana               `serix:"RequestManaAmount"`
}

type AccountState struct {
	AccountID  iotago.AccountID   `serix:""`
	PrivateKey ed25519.PrivateKey `serix:",lenPrefix=uint8"`
	OutputID   iotago.OutputID    `serix:""`
	Index      uint32             `serix:""`
}

func (a *AccountState) ToAccountData() *models.AccountData {
	return &models.AccountData{
		Account:  wallet.NewEd25519Account(a.AccountID, a.PrivateKey),
		OutputID: a.OutputID,
		Index:    a.Index,
	}
}

func AccountStateFromAccountData(acc *models.AccountData) AccountState {
	return AccountState{
		AccountID:  acc.Account.ID(),
		PrivateKey: acc.Account.PrivateKey(),
		OutputID:   acc.OutputID,
		Index:      acc.Index,
	}
}
