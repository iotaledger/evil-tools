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
func SplitBalanceEqually(splitNumber int, balance iotago.BaseToken) []iotago.BaseToken {
	outputBalances := make([]iotago.BaseToken, 0)

	// make sure the output balances are equal input
	var totalBalance iotago.BaseToken

	// input is divided equally among outputs
	for i := 0; i < splitNumber-1; i++ {
		outputBalances = append(outputBalances, balance/iotago.BaseToken(splitNumber))
		totalBalance += outputBalances[i]
	}
	lastBalance := balance - totalBalance
	outputBalances = append(outputBalances, lastBalance)

	return outputBalances
}

func SprintTransaction(tx *iotago.SignedTransaction) string {
	txDetails := ""
	txDetails += fmt.Sprintf("Transaction ID; %s, slotCreation: %d\n", lo.PanicOnErr(tx.ID()).ToHex(), tx.Transaction.CreationSlot)
	for index, out := range tx.Transaction.Outputs {
		txDetails += fmt.Sprintf("Output index: %d, base token: %d, stored mana: %d\n", index, out.BaseTokenAmount(), out.StoredMana())
	}
	txDetails += fmt.Sprintln("Allotments:")
	for _, allotment := range tx.Transaction.Allotments {
		txDetails += fmt.Sprintf("AllotmentID: %s, value: %d\n", allotment.AccountID, allotment.Mana)
	}
	for _, allotment := range tx.Transaction.TransactionEssence.Allotments {
		txDetails += fmt.Sprintf("al 2 AllotmentID: %s, value: %d\n", allotment.AccountID, allotment.Mana)
	}

	return txDetails
}

func SumOutputsBalance(outputs []*models.Output) iotago.BaseToken {
	balance := iotago.BaseToken(0)
	for _, out := range outputs {
		balance += out.Balance
	}

	if accountOutput {
		out := builder.NewAccountOutputBuilder(tpkg.RandAccountAddress(), 100).
			Mana(100).
			BlockIssuer(tpkg.RandomBlockIssuerKeysEd25519(1), iotago.MaxSlotIndex).MustBuild()
		txBuilder.AddOutput(out)
	}

	if accountInput || accountOutput {
		txBuilder.AddContextInput(&iotago.BlockIssuanceCreditInput{AccountID: iotago.EmptyAccountID})
		txBuilder.AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.EmptyCommitmentID})
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
