package accountmanager

import (
	"context"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-crypto-demo/pkg/bip32path"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/wallet"
)

func logMissingMana(log log.Logger, finishedTxBuilder *builder.TransactionBuilder, rmc iotago.Mana, issuerAccountID iotago.AccountID) {
	availableMana, err := finishedTxBuilder.CalculateAvailableManaInputs(finishedTxBuilder.CreationSlot())
	if err != nil {
		log.LogError("could not calculate available mana")

		return
	}
	log.LogDebug(utils.SprintAvailableManaResult(availableMana))
	minRequiredAllottedMana, err := finishedTxBuilder.MinRequiredAllottedMana(rmc, issuerAccountID)
	if err != nil {
		log.LogError("could not calculate min required allotted mana")

		return
	}
	log.LogDebugf("Min required allotted mana: %d", minRequiredAllottedMana)
}

// checkOutputStatus checks the status of an output by requesting all possible endpoints.
func (m *Manager) checkOutputStatus(ctx context.Context, clt *models.WebClient, blkID iotago.BlockID, txID iotago.TransactionID, creationOutputID iotago.OutputID, accountAddress *iotago.AccountAddress, checkIndexer ...bool) error {
	// request by blockID if provided, otherwise use txID
	slot := blkID.Slot()
	if blkID == iotago.EmptyBlockID {
		blkMetadata, err := clt.GetBlockStateFromTransaction(ctx, txID)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get block state from transaction %s", txID.ToHex())
		}
		blkID = blkMetadata.BlockID
		slot = blkMetadata.BlockID.Slot()
	}

	if err := utils.AwaitBlockAndPayloadAcceptance(ctx, m.Logger, clt, blkID); err != nil {
		return ierrors.Wrapf(err, "failed to await block issuance for block %s", blkID.ToHex())
	}
	m.LogInfof("Block and Transaction accepted: blockID %s", blkID.ToHex())

	// wait for the account to be committed
	if accountAddress != nil {
		m.LogInfof("Checking for commitment of account, blk ID: %s, txID: %s and creation output: %s\nBech addr: %s", blkID.ToHex(), txID.ToHex(), creationOutputID.ToHex(), accountAddress.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP()))
	} else {
		m.LogInfof("Checking for commitment of output, blk ID: %s, txID: %s and creation output: %s", blkID.ToHex(), txID.ToHex(), creationOutputID.ToHex())
	}
	err := utils.AwaitCommitment(ctx, m.Logger, clt, slot)
	if err != nil {
		m.LogErrorf("Failed to await commitment for slot %d: %s", slot, err)

		return err
	}

	// Check the indexer
	if len(checkIndexer) > 0 && checkIndexer[0] {
		outputID, account, _, err := clt.GetAccountFromIndexer(ctx, accountAddress)
		if err != nil {
			m.LogDebugf("Failed to get account from indexer, even after slot %d is already committed", slot)
			return ierrors.Wrapf(err, "failed to get account from indexer, even after slot %d is already committed", slot)
		}

		m.LogDebugf("Indexer returned: outputID %s, account %s, slot %d", outputID.String(), account.AccountID.ToAddress().Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP()), slot)
	}

	// check if the creation output exists
	outputFromNode, err := clt.Client().OutputByID(ctx, creationOutputID)
	if err != nil {
		m.LogDebugf("Failed to get output from node, even after slot %d is already committed", slot)
		return ierrors.Wrapf(err, "failed to get output from node, even after slot %d is already committed", slot)
	}
	m.LogDebugf("Node returned: outputID %s, output %s", creationOutputID.ToHex(), outputFromNode.Type())

	if accountAddress != nil {
		m.LogInfof("Account present in commitment for slot %d\nBech addr: %s", slot, accountAddress.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP()))
	} else {
		m.LogInfof("Output present in commitment for slot %d", slot)
	}

	return nil
}

func BIP32PathForIndex(index uint32) string {
	path := lo.PanicOnErr(bip32path.ParsePath(wallet.DefaultIOTAPath))
	if len(path) != 5 {
		panic("invalid path length")
	}

	// Set the index
	path[4] = index | (1 << 31)

	return path.String()
}
