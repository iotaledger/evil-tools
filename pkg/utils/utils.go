package utils

import (
	"fmt"

	"time"

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


// AwaitBlockToBeConfirmed awaits for acceptance of a single transaction.
func AwaitBlockToBeConfirmed(ctx context.Context, clt models.Client, blkID iotago.BlockID) error {
	for i := 0; i < MaxRetries; i++ {
		state := clt.GetBlockConfirmationState(ctx, blkID)
		if state == apimodels.BlockStateConfirmed.String() || state == apimodels.BlockStateFinalized.String() {
			UtilsLogger.Debugf("Block confirmed: %s", blkID.ToHex())
			return nil
		}

		time.Sleep(AwaitInterval)
	}

	UtilsLogger.Debugf("Block not confirmed: %s", blkID.ToHex())

	return ierrors.Errorf("Block not confirmed: %s", blkID.ToHex())
}

// AwaitTransactionToBeAccepted awaits for acceptance of a single transaction.
func AwaitTransactionToBeAccepted(ctx context.Context, clt models.Client, txID iotago.TransactionID) (string, error) {
	for i := 0; i < MaxRetries; i++ {
		resp, _ := clt.GetBlockStateFromTransaction(ctx, txID)
		if resp == nil {
			time.Sleep(AwaitInterval)

			continue
		}
		if resp.BlockState == apimodels.BlockStateFailed.String() || resp.BlockState == apimodels.BlockStateRejected.String() {
			failureReason, _, _ := apimodels.BlockFailureReasonFromBytes(lo.PanicOnErr(resp.BlockFailureReason.Bytes()))

			return resp.BlockState, ierrors.Errorf("tx %s failed because block failure: %d", txID, failureReason)
		}

		if resp.TransactionState == apimodels.TransactionStateFailed.String() {
			failureReason, _, _ := apimodels.TransactionFailureReasonFromBytes(lo.PanicOnErr(resp.TransactionFailureReason.Bytes()))
			UtilsLogger.Warnf("transaction %s failed: %d", txID, failureReason)

			return resp.TransactionState, ierrors.Errorf("transaction %s failed: %d", txID, failureReason)
		}

		confirmationState := resp.TransactionState
		if confirmationState == apimodels.TransactionStateAccepted.String() ||
			confirmationState == apimodels.TransactionStateConfirmed.String() ||
			confirmationState == apimodels.TransactionStateFinalized.String() {
			return confirmationState, nil
		}

		time.Sleep(AwaitInterval)
	}

	return "", ierrors.Errorf("Transaction %s not accepted in time", txID)
}

func AwaitAddressUnspentOutputToBeAccepted(ctx context.Context, clt models.Client, addr iotago.Address) (outputID iotago.OutputID, output iotago.Output, err error) {
	indexer, err := clt.Indexer(ctx)
	if err != nil {
		return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to get indexer client")
	}

	addrBech := addr.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP())

	for i := 0; i < MaxRetries; i++ {
		res, err := indexer.Outputs(ctx, &apimodels.BasicOutputsQuery{
			AddressBech32: addrBech,
		})
		if err != nil {
			return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "indexer request failed in request faucet funds")
		}

		for res.Next() {
			unspents, err := res.Outputs(ctx)
			if err != nil {
				return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to get faucet unspent outputs")
			}

			if len(unspents) == 0 {
				UtilsLogger.Debugf("no unspent outputs found in indexer for address: %s", addrBech)
				break
			}

			return lo.Return1(res.Response.Items.OutputIDs())[0], unspents[0], nil
		}

		time.Sleep(AwaitInterval)
	}

	return iotago.EmptyOutputID, nil, ierrors.Errorf("no unspent outputs found for address %s due to timeout", addrBech)
}

// AwaitOutputToBeAccepted awaits for output from a provided outputID is accepted. Timeout is waitFor.
// Useful when we have only an address and no transactionID, e.g. faucet funds request.
func AwaitOutputToBeAccepted(ctx context.Context, clt models.Client, outputID iotago.OutputID) bool {
	for i := 0; i < MaxRetries; i++ {
		confirmationState := clt.GetOutputConfirmationState(ctx, outputID)
		if confirmationState == apimodels.TransactionStateConfirmed.String() {
			return true
		}

		time.Sleep(AwaitInterval)
	}

	return false
}


func SprintTransaction(tx *iotago.SignedTransaction) string {
	txDetails := ""
	txDetails += fmt.Sprintf("\tTransaction ID; %s, slotCreation: %d\n", lo.PanicOnErr(tx.ID()).ToHex(), tx.Transaction.CreationSlot)
	for index, out := range tx.Transaction.TransactionEssence.Inputs {
		txDetails += fmt.Sprintf("\tInput index: %d, type: %s, ID: %s\n", index, out.Type())
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
		balance += out.Balance
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
