package utils

import (
	"context"
	"time"

	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

const (
	MaxRetries              = 20
	AwaitInterval           = 2 * time.Second
	MaxCommitmentAwait      = time.Minute
	AwaitCommitmentInterval = 5 * time.Second
)

// AwaitBlockToBeConfirmed awaits for acceptance of a single transaction.
func AwaitBlockToBeConfirmed(clt models.Client, blkID iotago.BlockID) error {
	for i := 0; i < MaxRetries; i++ {
		resp, err := clt.GetBlockConfirmationState(blkID)
		if err != nil {
			UtilsLogger.Debugf("Failed to get block confirmation state: %s", err)
			time.Sleep(AwaitInterval)

			continue
		}

		if resp.BlockState == apimodels.BlockStateConfirmed.String() || resp.BlockState == apimodels.BlockStateFinalized.String() {
			UtilsLogger.Debugf("Block confirmed: %s", blkID.ToHex())
			return nil
		}
		if resp.BlockState == apimodels.BlockStateFailed.String() || resp.BlockState == apimodels.BlockStateRejected.String() {
			UtilsLogger.Debugf("Block failed: %s", blkID.ToHex())
			return ierrors.Errorf("block %s failed", blkID.ToHex())
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

func AwaitCommitment(clt models.Client, slot iotago.SlotIndex) error {
	return ierrors.Wrapf(Await(MaxCommitmentAwait, AwaitCommitmentInterval, func() (bool, error) {
		resp, err := clt.GetBlockIssuance()
		if err != nil {
			return false, nil //nolint:nilerr
		}

		latestCommittedSlot := resp.Commitment.Slot
		if slot >= latestCommittedSlot {
			return true, nil
		}

		return false, nil
	}), "failed to await commitment for slot %d", slot)
}

func AwaitBlockIssuanceWithTransaction(clt models.Client, blockID iotago.BlockID) error {
	return ierrors.Wrapf(Await(MaxRetries*AwaitInterval, AwaitInterval, func() (bool, error) {
		resp, err := clt.GetBlockConfirmationState(blockID)
		if err != nil {
			return false, nil //nolint:nilerr
		}

		if resp.BlockState == apimodels.BlockStateConfirmed.String() || resp.BlockState == apimodels.BlockStateFinalized.String() {
			UtilsLogger.Debugf("Block confirmed: %s", blockID.ToHex())
			return true, nil
		}

		if resp.BlockState == apimodels.BlockStateFailed.String() || resp.BlockState == apimodels.BlockStateRejected.String() {
			UtilsLogger.Debugf("Block failed: %s", blockID.ToHex())
			return false, ierrors.Errorf("block %s failed", blockID.ToHex())
		}

		if resp.TransactionState == apimodels.TransactionStateFailed.String() {
			UtilsLogger.Debugf("Transaction in block failed: %s", blockID.ToHex())

			return false, ierrors.Errorf("transaction failed: %d: ", resp.TransactionFailureReason)
		}

		return false, nil
	}), "failed to await bloc confirmation or failure: %s", blockID.ToHex())
}

func Await(maxAwaitTime time.Duration, awaitInterval time.Duration, requestFunc func() (bool, error)) error {
	for t := time.Now(); time.Since(t) < maxAwaitTime; time.Sleep(awaitInterval) {
		done, err := requestFunc()
		if err != nil {
			return err
		}

		if done {
			return nil
		}
	}

	return ierrors.Errorf("await failed due to timeout")
}
