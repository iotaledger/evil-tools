package models

import (
	"crypto/ed25519"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
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
	PrivKey      ed25519.PrivateKey

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
	Account  mock.Account
	OutputID iotago.OutputID
	Index    uint64
}

type AccountState struct {
	Alias      string             `serix:"0,lengthPrefixType=uint8"`
	AccountID  iotago.AccountID   `serix:"2"`
	PrivateKey ed25519.PrivateKey `serix:"3,lengthPrefixType=uint8"`
	OutputID   iotago.OutputID    `serix:"4"`
	Index      uint64             `serix:"5"`
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
		Account:  mock.NewEd25519Account(a.AccountID, a.PrivateKey),
		OutputID: a.OutputID,
		Index:    a.Index,
	}
}

type PayloadIssuanceData struct {
	Payload            iotago.Payload
	CongestionResponse *apimodels.CongestionResponse
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
