package models

import (
	"crypto/ed25519"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/wallet"
)

// Input contains details of an input.
type Input struct {
	OutputID iotago.OutputID
	Address  iotago.Address
}

// Output contains details of an output ID.
type Output struct {
	OutputID     iotago.OutputID
	Address      iotago.Address
	AddressIndex uint64
	Balance      iotago.BaseToken
	PrivateKey   ed25519.PrivateKey

	OutputStruct iotago.Output
}

// Outputs is a list of Output.
type Outputs []*Output

type AccountStatus uint8

const (
	AccountPending AccountStatus = iota
	AccountReady
)

type AccountData struct {
	Alias    string
	Status   AccountStatus
	Account  wallet.Account
	OutputID iotago.OutputID
	Index    uint64
}

type AccountState struct {
	Alias      string             `serix:",lenPrefix=uint8"`
	AccountID  iotago.AccountID   `serix:""`
	PrivateKey ed25519.PrivateKey `serix:",lenPrefix=uint8"`
	OutputID   iotago.OutputID    `serix:""`
	Index      uint64             `serix:""`
}

func AccountStateFromAccountData(acc *AccountData) *AccountState {
	return &AccountState{
		Alias:      acc.Alias,
		AccountID:  acc.Account.ID(),
		PrivateKey: acc.Account.PrivateKey(),
		OutputID:   acc.OutputID,
		Index:      acc.Index,
	}
}

func (a *AccountState) ToAccountData() *AccountData {
	return &AccountData{
		Alias:    a.Alias,
		Account:  wallet.NewEd25519Account(a.AccountID, a.PrivateKey),
		OutputID: a.OutputID,
		Index:    a.Index,
	}
}

type PayloadIssuanceData struct {
	Payload            iotago.Payload
	CongestionResponse *api.CongestionResponse
}

type AllotmentStrategy uint8

const (
	AllotmentStrategyNone AllotmentStrategy = iota
	AllotmentStrategyMinCost
	AllotmentStrategyAll
)

type IssuancePaymentStrategy struct {
	AllotmentStrategy AllotmentStrategy
	IssuerAlias       string
}
