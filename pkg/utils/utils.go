package utils

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

const (
	MaxRetries    = 20
	AwaitInterval = 2 * time.Second
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
func AwaitBlockToBeConfirmed(clt models.Client, blkID iotago.BlockID) error {
	for i := 0; i < MaxRetries; i++ {
		state := clt.GetBlockConfirmationState(blkID)
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
func AwaitTransactionToBeAccepted(clt models.Client, txID iotago.TransactionID, txLeft *atomic.Int64) error {
	for i := 0; i < MaxRetries; i++ {
		resp, _ := clt.GetBlockStateFromTransaction(txID)
		if resp == nil {
			time.Sleep(AwaitInterval)

			continue
		}
		if resp.BlockState == apimodels.BlockStateFailed.String() || resp.BlockState == apimodels.BlockStateRejected.String() {
			failureReason, _, _ := apimodels.BlockFailureReasonFromBytes(lo.PanicOnErr(resp.BlockFailureReason.Bytes()))

			return ierrors.Errorf("tx %s failed because block failure: %d", txID, failureReason)
		}

		if resp.TransactionState == apimodels.TransactionStateFailed.String() {
			failureReason, _, _ := apimodels.TransactionFailureReasonFromBytes(lo.PanicOnErr(resp.TransactionFailureReason.Bytes()))
			UtilsLogger.Warnf("transaction %s failed: %d", txID, failureReason)

			return ierrors.Errorf("transaction %s failed: %d", txID, failureReason)
		}

		confirmationState := resp.TransactionState

		UtilsLogger.Debugf("Tx %s confirmationState: %s, tx left: %d", txID.ToHex(), confirmationState, txLeft.Load())
		if confirmationState == apimodels.TransactionStateAccepted.String() ||
			confirmationState == apimodels.TransactionStateConfirmed.String() ||
			confirmationState == apimodels.TransactionStateFinalized.String() {
			return nil
		}

		time.Sleep(AwaitInterval)
	}

	return ierrors.Errorf("Transaction %s not accepted in time", txID)
}

func AwaitAddressUnspentOutputToBeAccepted(clt models.Client, addr iotago.Address) (outputID iotago.OutputID, output iotago.Output, err error) {
	indexer, err := clt.Indexer()
	if err != nil {
		return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to get indexer client")
	}

	addrBech := addr.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP())

	for i := 0; i < MaxRetries; i++ {
		res, err := indexer.Outputs(context.Background(), &apimodels.BasicOutputsQuery{
			AddressBech32: addrBech,
		})
		if err != nil {
			return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "indexer request failed in request faucet funds")
		}

		for res.Next() {
			unspents, err := res.Outputs(context.TODO())
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
func AwaitOutputToBeAccepted(clt models.Client, outputID iotago.OutputID) bool {
	for i := 0; i < MaxRetries; i++ {
		confirmationState := clt.GetOutputConfirmationState(outputID)
		if confirmationState == apimodels.TransactionStateConfirmed.String() {
			return true
		}

		time.Sleep(AwaitInterval)
	}

	return false
}

func PrintTransaction(tx *iotago.SignedTransaction) string {
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
