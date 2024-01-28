package accountmanager

import (
	"context"
	"time"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
)

func (m *Manager) claim(ctx context.Context, params *ClaimAccountParams) error {
	delegations, err := m.GetDelegations(params.Alias)
	if err != nil {
		return ierrors.Wrapf(err, "could not get delegations for alias %s", params.Alias)
	}
	w, err := m.GetWallet(params.Alias)
	if err != nil {
		return ierrors.Wrapf(err, "could not get wallet and account for alias %s", params.Alias)
	}

	accData, err := m.GetAccount(params.Alias)
	if err != nil {
		return ierrors.Wrapf(err, "could not get account for alias %s", params.Alias)
	}

	delegationInputs := make([]*models.OutputData, 0)
	rewardsResponses := make([]*api.ManaRewardsResponse, 0)
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

		delegationInputs = append(delegationInputs, outputData)
		rewardsResponses = append(rewardsResponses, rewardsResp)
	}
	if len(delegationInputs) == 0 || len(rewardsResponses) == 0 {
		m.LogErrorf("no delegations found for alias %s", params.Alias)

		return nil
	}

	congestionResp, issuanceResp, version, err := m.RequestBlockIssuanceData(ctx, m.Client, m.GenesisAccount())
	if err != nil {
		return ierrors.Wrapf(err, "failed to request block issuance data for alias %s", params.Alias)
	}

	signedTx, err := m.createClaimingTransaction(delegationInputs, rewardsResponses, w, accData.Account.ID(), issuanceResp.LatestCommitment.MustID())
	if err != nil {
		return ierrors.Wrapf(err, "failed to create transaction with claiming to alias %s", params.Alias)
	}

	blkID, err := m.PostWithBlock(ctx, m.Client, signedTx, m.GenesisAccount(), congestionResp, issuanceResp, version)
	if err != nil {
		return ierrors.Wrapf(err, "failed to post transaction with claiming to alias %s", params.Alias)
	}

	m.LogInfof("Posted transaction with blockID %s: claiming rewards connected to alias %s", blkID.ToHex(), params.Alias)

	return nil
}
func (m *Manager) createClaimingTransaction(inputs []*models.OutputData, rewardsResponses []*api.ManaRewardsResponse, w *Wallet, accountID iotago.AccountID, commitmentID iotago.CommitmentID) (*iotago.SignedTransaction, error) {
	currentTime := time.Now()
	currentSlot := m.API.TimeProvider().SlotFromTime(currentTime)
	apiForSlot := m.Client.APIForSlot(currentSlot)

	// transaction signer
	addrSigner, err := w.GetAddrSignerForIndexes(inputs...)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get address signer")
	}

	txBuilder := builder.NewTransactionBuilder(apiForSlot)
	totalMana := iotago.Mana(0)
	for i, output := range inputs {
		potentialMana, err := iotago.PotentialMana(apiForSlot.ManaDecayProvider(), apiForSlot.StorageScoreStructure(), output.OutputStruct, output.OutputID.Slot(), currentSlot)
		if err != nil {
			return nil, ierrors.Wrap(err, "failed to get potential mana")
		}

		totalMana += potentialMana
		totalMana += rewardsResponses[i].Rewards
		totalMana += output.OutputStruct.StoredMana()
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: output.Address,
			InputID:      output.OutputID,
			Input:        output.OutputStruct,
		}).
			AddRewardInput(&iotago.RewardInput{Index: 0}, rewardsResponses[i].Rewards).
			AddCommitmentInput(&iotago.CommitmentInput{
				CommitmentID: commitmentID,
			})
	}

	totalBalance := utils.SumOutputsBalance(inputs)
	outputAddr, _, _ := w.getAddress(iotago.AddressEd25519)
	output := builder.NewBasicOutputBuilder(outputAddr, totalBalance).
		Mana(totalMana).
		MustBuild()

	txBuilder.
		AddOutput(output).
		SetCreationSlot(currentSlot).
		//WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
		AllotAllMana(txBuilder.CreationSlot(), accountID, 0)

	signedTx, err := txBuilder.Build(addrSigner)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to build tx")
	}

	return signedTx, nil

}
