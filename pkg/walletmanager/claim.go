package walletmanager

import (
	"context"
	"time"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/wallet"
)

func (m *Manager) claim(ctx context.Context, params *ClaimAccountParams) error {
	delegations, err := m.GetDelegations(params.Alias)
	if err != nil {
		return ierrors.Wrapf(err, "could not get delegations for alias %s", params.Alias)
	}
	w, err := m.GetWallet(params.Alias)
	if err != nil {
		return ierrors.Wrapf(err, "could not get wallet for alias %s", params.Alias)
	}

	// not all aliases have a corresponding account, as user could only delegate, in this case we allot to the genesis account
	var account wallet.Account
	accData, err := m.GetAccount(params.Alias)
	if err != nil {
		account = m.GenesisAccount()
	} else {
		account = accData.Account
	}

	for _, delegation := range delegations {
		rewardsResp, err := m.Client.GetRewards(ctx, delegation.OutputID)
		if err != nil {
			m.LogErrorf("failed to get rewards for outputID %s", delegation.OutputID.ToHex())

			continue
		}
		outputStruct := m.Client.GetOutput(ctx, delegation.OutputID)
		if outputStruct == nil {
			m.LogErrorf("failed to get output struct for outputID %s", delegation.OutputID.ToHex())

			continue
		}
		outputData := w.createOutputDataForIndex(delegation.OutputID, delegation.AddressIndex, outputStruct)

		congestionResp, issuanceResp, version, err := m.RequestBlockIssuanceData(ctx, m.Client, m.GenesisAccount())
		if err != nil {
			return ierrors.Wrapf(err, "failed to request block issuance data for alias %s", params.Alias)
		}

		signedTx, err := m.createClaimingTransaction(outputData, rewardsResp, w, account.ID(), issuanceResp.LatestCommitment.MustID())
		if err != nil {
			return ierrors.Wrapf(err, "failed to create transaction with claiming to alias %s", params.Alias)
		}

		blockID, err := m.PostWithBlock(ctx, m.Client, signedTx, m.GenesisAccount(), congestionResp, issuanceResp, version)
		if err != nil {
			return ierrors.Wrapf(err, "failed to post transaction with claiming to alias %s", params.Alias)
		}

		m.LogInfof("Posted transaction with blockID %s: claiming rewards connected to alias %s", blockID.ToHex(), params.Alias)
		m.LogInfof("Delegation output stored mana: %d, rewards amount according to API: %d", outputData.OutputStruct.StoredMana(), rewardsResp.Rewards)
		txID := lo.PanicOnErr(signedTx.Transaction.ID())
		basicOutputID := iotago.OutputIDFromTransactionIDAndIndex(txID, 0)

		if err := utils.AwaitBlockAndPayloadAcceptance(ctx, m.Logger, m.Client, blockID); err != nil {
			return ierrors.Wrapf(err, "failed to await block issuance for block %s", blockID.ToHex())
		}

		m.LogInfof("Block and Transaction accepted: blockID %s", blockID.ToHex())
		// check if the creation output exists
		out, err := m.Client.Client().OutputByID(ctx, basicOutputID)
		if err != nil {
			m.LogDebugf("Failed to get output from node")

			return ierrors.Wrapf(err, "failed to get output from node")
		}
		m.LogInfof("Output with reward exists, with stored mana: %d", out.StoredMana())
	}

	m.removeDelegations(params.Alias)

	return nil
}
func (m *Manager) createClaimingTransaction(input *models.OutputData, rewardsResponse *api.ManaRewardsResponse, w *Wallet, accountID iotago.AccountID, commitmentID iotago.CommitmentID) (*iotago.SignedTransaction, error) {
	currentTime := time.Now()
	currentSlot := m.API.TimeProvider().SlotFromTime(currentTime)
	apiForSlot := m.Client.APIForSlot(currentSlot)

	// transaction signer
	addrSigner, err := w.GetAddrSignerForIndexes(input)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get address signer")
	}

	txBuilder := builder.NewTransactionBuilder(apiForSlot)
	totalMana := iotago.Mana(0)
	potentialMana, err := iotago.PotentialMana(apiForSlot.ManaDecayProvider(), apiForSlot.StorageScoreStructure(), input.OutputStruct, input.OutputID.Slot(), currentSlot)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get potential mana")
	}

	totalMana += potentialMana
	totalMana += rewardsResponse.Rewards
	totalMana += input.OutputStruct.StoredMana()
	txBuilder.AddInput(&builder.TxInput{
		UnlockTarget: input.Address,
		InputID:      input.OutputID,
		Input:        input.OutputStruct,
	}).
		AddRewardInput(&iotago.RewardInput{Index: 0}, rewardsResponse.Rewards).
		AddCommitmentInput(&iotago.CommitmentInput{
			CommitmentID: commitmentID,
		})

	totalBalance := input.OutputStruct.BaseTokenAmount()
	outputAddr, _, _ := w.getAddress(iotago.AddressEd25519)
	output := builder.NewBasicOutputBuilder(outputAddr, totalBalance).
		Mana(totalMana).
		MustBuild()

	txBuilder.
		AddOutput(output).
		SetCreationSlot(currentSlot).
		AllotAllMana(txBuilder.CreationSlot(), accountID, 0)

	signedTx, err := txBuilder.Build(addrSigner)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to build tx")
	}

	return signedTx, nil

}
