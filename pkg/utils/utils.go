package utils

import (
	"crypto"
	"crypto/ed25519"

	"github.com/iotaledger/evil-tools/pkg/models"
	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

func DelegationEnd(apiForSlot iotago.API, issuingSlot iotago.SlotIndex, commitmentSlot iotago.SlotIndex) iotago.EpochIndex {
	futureBoundedSlotIndex := commitmentSlot + apiForSlot.ProtocolParameters().MinCommittableAge()
	futureBoundedEpochIndex := apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex)

	registrationSlot := DelegationRegistrationSlot(apiForSlot, issuingSlot)

	if futureBoundedEpochIndex <= iotago.EpochIndex(registrationSlot) {
		return futureBoundedEpochIndex
	}

	return futureBoundedEpochIndex + 1
}

func DelegationRegistrationSlot(apiForSlot iotago.API, slot iotago.SlotIndex) iotago.SlotIndex {
	return apiForSlot.TimeProvider().EpochEnd(apiForSlot.TimeProvider().EpochFromSlot(slot)) - apiForSlot.ProtocolParameters().EpochNearingThreshold()
}

func GetAccountIssuerKeys(pubKey crypto.PublicKey) (iotago.BlockIssuerKeys, error) {
	ed25519PubKey, isEd25519 := pubKey.(ed25519.PublicKey)
	if !isEd25519 {
		return nil, ierrors.New("Failed to create account: only Ed25519 keys are supported")
	}

	blockIssuerKeys := iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(hiveEd25519.PublicKey(ed25519PubKey)))

	return blockIssuerKeys, nil

}

// SplitBalanceEqually splits the balance equally between `splitNumber` outputs.
func SplitBalanceEqually[T iotago.BaseToken | iotago.Mana](splitNumber int, balance T) []T {
	outputBalances := make([]T, 0)

	// make sure the output balances are equal input
	var totalBalance T

	// input is divided equally among outputs
	for i := 0; i < splitNumber-1; i++ {
		outputBalances = append(outputBalances, balance/T(splitNumber))
		totalBalance += outputBalances[i]
	}
	lastBalance := balance - totalBalance
	outputBalances = append(outputBalances, lastBalance)

	return outputBalances
}

func SumOutputsBalance(outputs []*models.OutputData) iotago.BaseToken {
	balance := iotago.BaseToken(0)
	for _, out := range outputs {
		balance += out.OutputStruct.BaseTokenAmount()
	}

	return balance
}
