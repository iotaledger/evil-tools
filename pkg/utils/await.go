package utils

import (
	"context"
	"time"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

const (
	MaxAcceptanceAwait = 90 * time.Second
	AwaitInterval      = 1 * time.Second

	AwaitCommitmentInterval = 10 * time.Second
)

func isBlockStateAtLeastAccepted(blockState api.BlockState) bool {
	return blockState == api.BlockStateAccepted ||
		blockState == api.BlockStateConfirmed ||
		blockState == api.BlockStateFinalized
}

func isTransactionStateAtLeastAccepted(transactionState api.TransactionState) bool {
	return transactionState == api.TransactionStateAccepted ||
		transactionState == api.TransactionStateCommitted ||
		transactionState == api.TransactionStateFinalized
}

func isBlockStateFailure(blockState api.BlockState) bool {
	return blockState == api.BlockStateDropped ||
		blockState == api.BlockStateOrphaned
}

func isTransactionStateFailure(transactionState api.TransactionState) bool {
	return transactionState == api.TransactionStateFailed
}

func evaluateBlockIssuanceResponse(resp *api.BlockMetadataResponse) (accepted bool, err error) {
	if isBlockStateAtLeastAccepted(resp.BlockState) {
		return true, nil
	}

	if isBlockStateFailure(resp.BlockState) {
		err = ierrors.Errorf("block status failure")
		if isBlockStateFailure(resp.BlockState) {
			err = ierrors.Wrapf(err, "block failure reason: %s", resp.BlockState.String())
		}

		return false, err
	}

	return false, nil
}

func evaluateTransactionIssuanceResponse(resp *api.TransactionMetadataResponse) (accepted bool, err error) {
	if isTransactionStateAtLeastAccepted(resp.TransactionState) {
		return true, nil
	}

	if isTransactionStateFailure(resp.TransactionState) {
		return false, ierrors.Wrapf(err, "transaction failure reason: %d", resp.TransactionFailureReason)
	}

	return false, nil
}

// AwaitBlockAndPayloadAcceptance waits for the block and, if provided, tx to be accepted.
func AwaitBlockAndPayloadAcceptance(ctx context.Context, logger log.Logger, clt models.Client, blockID iotago.BlockID) error {
	for t := time.Now(); time.Since(t) < MaxAcceptanceAwait; time.Sleep(AwaitInterval) {
		resp, err := clt.GetBlockConfirmationState(ctx, blockID)
		if err != nil {
			logger.LogDebugf("Failed to get block confirmation state: %s", err)

			continue
		}

		accepted, err := evaluateBlockIssuanceResponse(resp)
		if accepted {
			logger.LogDebugf("Block %s issuance success, status: %s", blockID.ToHex(), resp.BlockState)

			return nil
		}

		if err != nil {
			logger.LogDebugf("Block %s issuance failure, block failure reason: %s", blockID.ToHex(), resp.BlockState.String())

			return err
		}
	}

	return ierrors.Errorf("failed to await block confirmation or failure: %s", blockID.ToHex())
}

// AwaitBlockWithTransactionToBeAccepted awaits for acceptance of a single transaction.
func AwaitBlockWithTransactionToBeAccepted(ctx context.Context, logger log.Logger, clt models.Client, txID iotago.TransactionID) error {
	for t := time.Now(); time.Since(t) < MaxAcceptanceAwait; time.Sleep(AwaitInterval) {
		resp, _ := clt.GetTransactionMetadata(ctx, txID)
		if resp == nil {
			continue
		}

		accepted, err := evaluateTransactionIssuanceResponse(resp)
		if accepted {
			logger.LogDebugf("Transaction %s issuance success, state: %s", txID.ToHex(), resp.TransactionState)

			return nil
		}

		if err != nil {
			logger.LogDebugf("Transaction %s issuance failure, tx failure reason: %d", txID.ToHex(), resp.TransactionFailureReason)

			return err
		}
	}

	return ierrors.Errorf("Transaction %s not accepted in time", txID)
}

// AwaitAddressUnspentOutputToBeAccepted awaits for acceptance of an output created for an address, based on the status of the transaction.
func AwaitAddressUnspentOutputToBeAccepted(ctx context.Context, logger log.Logger, clt models.Client, addr iotago.Address) (outputID iotago.OutputID, output iotago.Output, err error) {
	addrBech := addr.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP())

	for t := time.Now(); time.Since(t) < MaxAcceptanceAwait; time.Sleep(AwaitInterval) {
		res, err := clt.Indexer().Outputs(ctx, &api.BasicOutputsQuery{
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
				logger.LogDebugf("no unspent outputs found in indexer for address: %s", addrBech)
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
		resp, err := clt.GetTransactionMetadata(ctx, outputID.TransactionID())
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
func AwaitCommitment(ctx context.Context, logger log.Logger, clt models.Client, targetSlot iotago.SlotIndex) error {
	currentCommittedSlot, err := getLatestCommittedSlot(ctx, clt)
	if err != nil {
		return ierrors.Wrap(err, "failed to get node info")
	}
	if targetSlot <= currentCommittedSlot {
		return nil
	}
	for t := currentCommittedSlot; t <= targetSlot; t++ {
		latestCommittedSlot, err := getLatestCommittedSlot(ctx, clt)
		if err != nil {
			return ierrors.Wrap(err, "failed to get node info")
		}

		if targetSlot <= latestCommittedSlot {
			return nil
		}

		logger.LogDebugf("Awaiting commitment for slot %d, latest committed slot: %d", targetSlot, latestCommittedSlot)
		time.Sleep(AwaitCommitmentInterval)
	}

	return ierrors.Errorf("failed to await commitment for slot %d", targetSlot)
}

func getLatestCommittedSlot(ctx context.Context, clt models.Client) (iotago.SlotIndex, error) {
	resp, err := clt.Client().Info(ctx)
	if err != nil {
		return 0, ierrors.Wrap(err, "failed to get node info")
	}

	return resp.Status.LatestCommitmentID.Slot(), nil
}
