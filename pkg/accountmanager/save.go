package accountmanager

import (
	"crypto/ed25519"
	"os"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/wallet"
)

func (m *Manager) toAccountState() *AccountsState {
	state := &AccountsState{
		AccountState:       make([]*AccountState, len(m.accounts)),
		Wallets:            make([]*Wallet, len(m.wallets)),
		Delegation:         make([]*Delegation, 0),
		RequestTokenAmount: m.RequestTokenAmount,
		RequestManaAmount:  m.RequestManaAmount,
	}

	for i, accountData := range lo.Values(m.accounts) {
		state.AccountState[i] = AccountStateFromAccountData(accountData)
	}

	for i, w := range lo.Values(m.wallets) {
		state.Wallets[i] = w
	}

	for _, delegations := range lo.Values(m.delegations) {
		state.Delegation = append(state.Delegation, delegations...)
	}

	return state
}

func (m *Manager) fromAccountState(state *AccountsState) {
	for _, accState := range state.AccountState {
		m.accounts[accState.Alias] = accState.ToAccountData()
	}
	for _, w := range state.Wallets {
		m.wallets[w.Alias] = w
	}

	for _, d := range state.Delegation {
		if _, ok := m.delegations[d.Alias]; !ok {
			m.delegations[d.Alias] = make([]*Delegation, 0)
		}
		m.delegations[d.Alias] = append(m.delegations[d.Alias], d)
	}

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

	m.LogInfof("Wallet saved successfully to %s file...", m.optsAccountStatesFile)

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

	state := &AccountsState{
		AccountState: make([]*AccountState, 0),
		Wallets:      make([]*Wallet, 0),
		Delegation:   make([]*Delegation, 0),
	}
	_, err = m.Client.LatestAPI().Decode(walletStateBytes, state)
	if err != nil {
		return false, ierrors.Wrap(err, "failed to decode from file")
	}
	m.fromAccountState(state)

	return true, nil
}

type AccountsState struct {
	AccountState       []*AccountState  `serix:"accounts,lenPrefix=uint8"`
	Wallets            []*Wallet        `serix:"wallets,lenPrefix=uint8"`
	Delegation         []*Delegation    `serix:"delegations,lenPrefix=uint8"`
	RequestTokenAmount iotago.BaseToken `serix:"RequestTokenAmount"`
	RequestManaAmount  iotago.Mana      `serix:"RequestManaAmount"`
}

type AccountState struct {
	Alias      string             `serix:"Alias,lenPrefix=uint8"`
	AccountID  iotago.AccountID   `serix:"AccountID"`
	PrivateKey ed25519.PrivateKey `serix:",lenPrefix=uint8"`
	OutputID   iotago.OutputID    `serix:"OutputID"`
	Index      uint32             `serix:"Index"`
}

type Delegation struct {
	Alias                  string           `serix:"Alias,lenPrefix=uint8"`
	OutputID               iotago.OutputID  `serix:"OutputID"`
	AddressIndex           uint32           `serix:"AddressIndex"`
	Amount                 iotago.BaseToken `serix:"Amount"`
	DelegatedToBechAddress string           `serix:"DelegatedToBechAddress,lenPrefix=uint8"`
}

func (a *AccountState) ToAccountData() *models.AccountData {
	return &models.AccountData{
		Account:  wallet.NewEd25519Account(a.AccountID, a.PrivateKey),
		OutputID: a.OutputID,
		Index:    a.Index,
	}
}

func AccountStateFromAccountData(acc *models.AccountData) *AccountState {
	return &AccountState{
		AccountID:  acc.Account.ID(),
		PrivateKey: acc.Account.PrivateKey(),
		OutputID:   acc.OutputID,
		Index:      acc.Index,
	}
}
