package utils

import (
	"fmt"

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

func SprintTransaction(tx *iotago.SignedTransaction) string {
	txDetails := ""
	txDetails += fmt.Sprintf("\tSigned Transaction ID: %s, txID: %s, slotCreation: %d\n", lo.PanicOnErr(tx.ID()).ToHex(), lo.PanicOnErr(tx.Transaction.ID()).ToHex(), tx.Transaction.CreationSlot)
	for index, out := range tx.Transaction.TransactionEssence.Inputs {
		txDetails += fmt.Sprintf("\tInput index: %d, type: %s\n", index, out.Type())
	}
	for _, out := range tx.Transaction.TransactionEssence.ContextInputs {
		txDetails += fmt.Sprintf("\tContext input: %s\n", out.Type())
	}
	for index, out := range tx.Transaction.Outputs {
		txDetails += fmt.Sprintf("\tOutput index: %d, base token: %d, stored mana: %d, type: %s\n", index, out.BaseTokenAmount(), out.StoredMana(), out.Type())
	}
	txDetails += fmt.Sprintln("\tAllotments:")
	for _, allotment := range tx.Transaction.Allotments {
		txDetails += fmt.Sprintf("\tAllotmentID: %s, value: %d\n", allotment.AccountID, allotment.Mana)
	}

	return txDetails
}

func SumOutputsBalance(outputs []*models.Output) iotago.BaseToken {
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
			BlockIssuer(tpkg.RandomBlockIssuerKeysEd25519(1), iotago.MaxSlotIndex).MustBuild()
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: tpkg.RandEd25519Address(),
			InputID:      iotago.EmptyOutputID,
			Input:        out,
		})
	}

	if accountOutput {
		out := builder.NewAccountOutputBuilder(tpkg.RandAccountAddress(), 100).
			Mana(100).
			BlockIssuer(tpkg.RandomBlockIssuerKeysEd25519(1), iotago.MaxSlotIndex).MustBuild()
		txBuilder.AddOutput(out)
	}

	if accountInput || accountOutput {
		txBuilder.AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: iotago.EmptyAccountID})
		txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: iotago.EmptyCommitmentID})
	}

	return txBuilder
}

func SprintAccount(acc *iotago.AccountOutput) string {
	accountStr := ""
	accountStr += fmt.Sprintf("Account ID: %s\n", acc.AccountID.ToHex())
	accountStr += fmt.Sprintf("Account token balance: %d\n", acc.Amount)
	accountStr += fmt.Sprintf("Account stored mana: %d\n", acc.Mana)

	blockIssuerFeature := acc.FeatureSet().BlockIssuer()
	if blockIssuerFeature != nil {
		accountStr += fmt.Sprintf("Block Issuer Feature, number of keys: %d\n", len(blockIssuerFeature.BlockIssuerKeys))
	}
	stakingFeature := acc.FeatureSet().Staking()
	if stakingFeature != nil {
		accountStr += "Staking Feature, number of keys:\n"
		accountStr += fmt.Sprintf("Staked Amount: %d, Fixed Cost: %d, Start Epoch Start: %d, End Epoch: %d", stakingFeature.StakedAmount, stakingFeature.FixedCost, stakingFeature.StartEpoch, stakingFeature.EndEpoch)
	}

	return accountStr
}

func SprintAvailableManaResult(results *builder.AvailableManaResult) string {
	return fmt.Sprintf("Available mana results:\nTotal: %d Unbound: %d\nPotential:%d Unbound: %d\nStored: %d Undound: %d", results.TotalMana, results.UnboundMana, results.PotentialMana, results.UnboundPotentialMana, results.StoredMana, results.UnboundStoredMana)
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
					&iotago.Ed25519PublicKeyBlockIssuerKey{},
				},
			},
		},
		ImmutableFeatures: iotago.AccountOutputImmFeatures{},
	}
	storageScoreStructure := iotago.NewStorageScoreStructure(protocolParameters.StorageScoreParameters())

	return lo.PanicOnErr(storageScoreStructure.MinDeposit(dummyAccountOutput))
}
