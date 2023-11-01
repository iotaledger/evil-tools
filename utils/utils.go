package utils

import (
	"context"
	"time"

	"go.uber.org/atomic"

	evillogger "github.com/iotaledger/evil-tools/logger"
	"github.com/iotaledger/evil-tools/models"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

var UtilsLogger = evillogger.New("Utils")

const (
	MaxRetries    = 20
	AwaitInterval = 1 * time.Second

	confirmed = "confirmed"
	finalized = "finalized"
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

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////////

// AwaitTransactionToBeAccepted awaits for acceptance of a single transaction.
func AwaitBlockToBeConfirmed(clt models.Client, blkID iotago.BlockID) error {
	for i := 0; i < MaxRetries; i++ {
		state := clt.GetBlockConfirmationState(blkID)
		if state == confirmed || state == finalized {
			return nil
		}

		time.Sleep(AwaitInterval)
	}

	UtilsLogger.Debugf("Block not confirmed: %s", blkID.ToHex())

	return ierrors.Errorf("Block not confirmed: %s", blkID.ToHex())
}

// AwaitTransactionToBeAccepted awaits for acceptance of a single transaction.
func AwaitTransactionToBeAccepted(clt models.Client, txID iotago.TransactionID, txLeft *atomic.Int64) error {
	var accepted bool
	for i := 0; i < MaxRetries; i++ {
		resp, err := clt.GetBlockState(txID)
		if resp == nil {
			UtilsLogger.Debugf("Block state API error: %v", err)

			continue
		}
		if resp.BlockState == apimodels.BlockStateFailed.String() || resp.BlockState == apimodels.BlockStateRejected.String() {
			failureReason, _, _ := apimodels.BlockFailureReasonFromBytes(lo.PanicOnErr(resp.BlockFailureReason.Bytes()))

			return ierrors.Errorf("tx %s failed because block failure: %d", txID, failureReason)
		}

		if resp.TransactionState == apimodels.TransactionStateFailed.String() {
			failureReason, _, _ := apimodels.TransactionFailureReasonFromBytes(lo.PanicOnErr(resp.TransactionFailureReason.Bytes()))

			return ierrors.Errorf("transaction %s failed: %d", txID, failureReason)
		}

		confirmationState := resp.TransactionState

		UtilsLogger.Debugf("Tx %s confirmationState: %s, tx left: %d", txID.ToHex(), confirmationState, txLeft.Load())
		if confirmationState == "accepted" || confirmationState == "confirmed" || confirmationState == "finalized" {
			accepted = true
			break
		}

		time.Sleep(AwaitInterval)
	}
	if !accepted {
		return ierrors.Errorf("transaction %s not accepted in time", txID)
	}

	UtilsLogger.Debugf("Transaction %s accepted", txID)

	return nil
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
func AwaitOutputToBeAccepted(clt models.Client, outputID iotago.OutputID) (accepted bool) {
	accepted = false
	for i := 0; i < MaxRetries; i++ {
		confirmationState := clt.GetOutputConfirmationState(outputID)
		if confirmationState == "confirmed" {
			accepted = true
			break
		}

		time.Sleep(AwaitInterval)
	}

	return accepted
}
