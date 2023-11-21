package utils

import (
	"context"
	"time"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

const (
	MaxAcceptanceAwait = 30 * time.Second
	AwaitInterval      = 2 * time.Second

	MaxCommitmentAwait      = 90 * time.Second
	AwaitCommitmentInterval = 10 * time.Second
)

func isBlockStateAtLeastAccepted(blockState string) bool {
	return blockState == apimodels.BlockStateAccepted.String() ||
		blockState == apimodels.BlockStateConfirmed.String() ||
		blockState == apimodels.BlockStateFinalized.String()
}

func isTransactionStateAtLeastAccepted(transactionState string) bool {
	return transactionState == apimodels.TransactionStateAccepted.String() ||
		transactionState == apimodels.TransactionStateConfirmed.String() ||
		transactionState == apimodels.TransactionStateFinalized.String()
}

func isBlockStateFailure(blockState string) bool {
	return blockState == apimodels.BlockStateFailed.String() ||
		blockState == apimodels.BlockStateRejected.String()
}

func isTransactionStateFailure(transactionState string) bool {
	return transactionState == apimodels.TransactionStateFailed.String()
}

func evaluateBlockIssuanceResponse(resp *apimodels.BlockMetadataResponse) (accepted bool, err error) {
	if isBlockStateAtLeastAccepted(resp.BlockState) && isTransactionStateAtLeastAccepted(resp.TransactionState) {
		return true, nil
	}

	if isBlockStateFailure(resp.BlockState) || isTransactionStateFailure(resp.TransactionState) {
		err = ierrors.Errorf("block status failure")
		if isBlockStateFailure(resp.BlockState) {
			err = ierrors.Wrapf(err, "block failure reason: %d", resp.BlockFailureReason)
		}
		if isTransactionStateFailure(resp.TransactionState) {
			err = ierrors.Wrapf(err, "transaction failure reason: %d", resp.TransactionFailureReason)
		}

		return false, err
	}

	return false, nil
}

// AwaitBlockAndPayloadAcceptance waits for the block and, if provided, tx to be accepted.
func AwaitBlockAndPayloadAcceptance(ctx context.Context, clt models.Client, blockID iotago.BlockID) error {
	for t := time.Now(); time.Since(t) < MaxAcceptanceAwait; time.Sleep(AwaitInterval) {
		resp, err := clt.GetBlockConfirmationState(ctx, blockID)
		if err != nil {
			Logger.Debugf("Failed to get block confirmation state: %s", err)

			continue
		}

		accepted, err := evaluateBlockIssuanceResponse(resp)
		if accepted {
			Logger.Debugf("Block %s issuance success, status: %s, transaction state: %s", blockID.ToHex(), resp.BlockState, resp.TransactionState)

			return nil
		}

		if err != nil {
			Logger.Debugf("Block %s issuance failure, block failure reason: %d, tx failure reason: %d", blockID.ToHex(), resp.BlockFailureReason, resp.TransactionFailureReason)

			return err
		}
	}

	return ierrors.Errorf("failed to await block confirmation or failure: %s", blockID.ToHex())
}

// AwaitBlockWithTransactionToBeAccepted awaits for acceptance of a single transaction.
func AwaitBlockWithTransactionToBeAccepted(ctx context.Context, clt models.Client, txID iotago.TransactionID) error {
	for t := time.Now(); time.Since(t) < MaxAcceptanceAwait; time.Sleep(AwaitInterval) {
		resp, _ := clt.GetBlockStateFromTransaction(ctx, txID)
		if resp == nil {
			continue
		}

		accepted, err := evaluateBlockIssuanceResponse(resp)
		if accepted {
			Logger.Debugf("Transaction %s issuance success, state: %s", txID.ToHex(), resp.TransactionState)

			return nil
		}

		if err != nil {
			Logger.Debugf("Transaction %s issuance failure, tx failure reason: %d, block failure reason: %d", txID.ToHex(), resp.TransactionFailureReason, resp.BlockFailureReason)

			return err
		}

	}

	return ierrors.Errorf("Transaction %s not accepted in time", txID)
}

// AwaitAddressUnspentOutputToBeAccepted awaits for acceptance of an output created for an address, based on the status of the transaction.
func AwaitAddressUnspentOutputToBeAccepted(ctx context.Context, clt models.Client, addr iotago.Address) (outputID iotago.OutputID, output iotago.Output, err error) {
	indexer, err := clt.Indexer(ctx)
	if err != nil {
		return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to get indexer client")
	}

	addrBech := addr.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP())

	for t := time.Now(); time.Since(t) < MaxAcceptanceAwait; time.Sleep(AwaitInterval) {
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
				Logger.Debugf("no unspent outputs found in indexer for address: %s", addrBech)
				break
			}

			return lo.Return1(res.Response.Items.OutputIDs())[0], unspents[0], nil
		}
	}

	return iotago.EmptyOutputID, nil, ierrors.Errorf("no unspent outputs found for address %s due to timeout", addrBech)
}

// AwaitOutputToBeAccepted awaits for output from a provided outputID is accepted. Timeout is waitFor.
// Useful when we have only an address and no transactionID, e.g. faucet funds request.
func AwaitOutputToBeAccepted(ctx context.Context, clt models.Client, outputID iotago.OutputID) error {
	for t := time.Now(); time.Since(t) < MaxAcceptanceAwait; time.Sleep(AwaitInterval) {
		resp, err := clt.GetBlockStateFromTransaction(ctx, outputID.TransactionID())
		if err != nil {
			continue
		}

		if isTransactionStateAtLeastAccepted(resp.TransactionState) {
			return nil
		}
	}

	return ierrors.Errorf("failed to await output %s to be accepted", outputID)
}

// AwaitCommitment awaits for the commitment of a slot.
func AwaitCommitment(ctx context.Context, clt models.Client, slot iotago.SlotIndex) error {
	for t := time.Now(); time.Since(t) < MaxCommitmentAwait; time.Sleep(AwaitCommitmentInterval) {
		resp, err := clt.GetBlockIssuance(ctx)
		if err != nil {
			continue
		}
		Logger.Debugf("Awaiting commitment for slot %d, latest committed slot: %d", slot, resp.Commitment.Slot)

		latestCommittedSlot := resp.Commitment.Slot
		if slot <= latestCommittedSlot {
			return nil
		}
	}

	return ierrors.Errorf("failed to await commitment for slot %d", slot)
}
