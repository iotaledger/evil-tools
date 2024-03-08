package models

import (
	"crypto/ed25519"

	"golang.org/x/crypto/blake2b"

	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/hexutil"
	"github.com/iotaledger/iota.go/v4/wallet"
)

// Input contains details of an input.
type Input struct {
	OutputID iotago.OutputID
	Address  iotago.Address
}

// OutputData contains details of an output ID.
type OutputData struct {
	OutputID     iotago.OutputID
	TempID       TempOutputID
	Address      iotago.Address
	AddressIndex uint32
	PrivateKey   ed25519.PrivateKey

	OutputStruct iotago.Output
}

func NewOutputDataWithEmptyID(api iotago.API, addr iotago.Address, index uint32, privateKey ed25519.PrivateKey, out iotago.Output) (*OutputData, error) {
	outID, err := NewTempOutputID(api, out)
	if err != nil {
		return nil, err
	}

	return &OutputData{
		OutputID:     iotago.EmptyOutputID,
		TempID:       outID,
		Address:      addr,
		AddressIndex: index,
		PrivateKey:   privateKey,
		OutputStruct: out,
	}, nil
}

func NewOutputDataWithID(api iotago.API, outputID iotago.OutputID, addr iotago.Address, index uint32, privateKey ed25519.PrivateKey, out iotago.Output) (*OutputData, error) {
	tempID, err := NewTempOutputID(api, out)
	if err != nil {
		return nil, err
	}

	return &OutputData{
		OutputID:     outputID,
		TempID:       tempID,
		Address:      addr,
		AddressIndex: index,
		PrivateKey:   privateKey,
		OutputStruct: out,
	}, nil
}

// Outputs is a list of Output.
type Outputs []*OutputData

type AccountStatus uint8

const (
	AccountPending AccountStatus = iota
	AccountReady
)

type AccountData struct {
	Status   AccountStatus
	Account  wallet.Account
	OutputID iotago.OutputID
	Index    uint32
}

// PayloadIssuanceData contains data for issuing a payload. Either ready payload or transaction build data along with issuer account required for signing.
type PayloadIssuanceData struct {
	Type               iotago.PayloadType
	TransactionBuilder *builder.TransactionBuilder
	Payload            iotago.Payload
}

type TempOutputID [32]byte

var EmptyTempOutputID = TempOutputID{}

func NewTempOutputID(api iotago.API, out iotago.Output) (TempOutputID, error) {
	b, err := api.Encode(out)
	if err != nil {
		return EmptyTempOutputID, err
	}

	return blake2b.Sum256(b), nil
}

func (f TempOutputID) Bytes() []byte {
	return f[:]
}

func (f TempOutputID) String() string {
	return hexutil.EncodeHex(f[:])
}
