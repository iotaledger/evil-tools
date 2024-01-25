package utils

import (
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

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

func PrepareDummyTransactionBuilder(api iotago.API, basicInputCount, basicOutputCount int, accountInput bool, accountOutput bool) *builder.TransactionBuilder {
	txBuilder := builder.NewTransactionBuilder(api)
	txBuilder.SetCreationSlot(100)
	for i := 0; i < basicInputCount; i++ {
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: tpkg.RandEd25519Address(),
			InputID:      iotago.EmptyOutputID,
			Input:        tpkg.RandBasicOutput(iotago.AddressEd25519),
		})
	}
	for i := 0; i < basicOutputCount; i++ {
		txBuilder.AddOutput(tpkg.RandBasicOutput(iotago.AddressEd25519))
	}

	if accountInput {
		out := builder.NewAccountOutputBuilder(tpkg.RandAccountAddress(), 100).
			Mana(100).
			BlockIssuer(tpkg.RandBlockIssuerKeys(1), iotago.MaxSlotIndex).MustBuild()
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: tpkg.RandEd25519Address(),
			InputID:      iotago.EmptyOutputID,
			Input:        out,
		})
	}

	if accountOutput {
		out := builder.NewAccountOutputBuilder(tpkg.RandAccountAddress(), 100).
			Mana(100).
			BlockIssuer(tpkg.RandBlockIssuerKeys(1), iotago.MaxSlotIndex).MustBuild()
		txBuilder.AddOutput(out)
	}

	if accountInput || accountOutput {
		txBuilder.AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: iotago.EmptyAccountID})
		txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: iotago.EmptyCommitmentID})
	}

	return txBuilder
}

func MinIssuerAccountAmount(protocolParameters iotago.ProtocolParameters) iotago.BaseToken {
	// create a dummy account with a block issuer feature to calculate the storage score.
	dummyAccountOutput := &iotago.AccountOutput{
		Amount:         0,
		Mana:           0,
		AccountID:      iotago.EmptyAccountID,
		FoundryCounter: 0,
		UnlockConditions: iotago.AccountOutputUnlockConditions{
			&iotago.AddressUnlockCondition{
				Address: &iotago.Ed25519Address{},
			},
		},
		Features: iotago.AccountOutputFeatures{
			&iotago.BlockIssuerFeature{
				BlockIssuerKeys: iotago.BlockIssuerKeys{
					&iotago.Ed25519PublicKeyHashBlockIssuerKey{},
				},
			},
		},
		ImmutableFeatures: iotago.AccountOutputImmFeatures{},
	}
	storageScoreStructure := iotago.NewStorageScoreStructure(protocolParameters.StorageScoreParameters())

	return lo.PanicOnErr(storageScoreStructure.MinDeposit(dummyAccountOutput))
}
