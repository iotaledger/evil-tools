package accountwallet

import (
	"context"
	"time"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

func (a *AccountWallet) delegateToAccount(ctx context.Context, params *DelegateAccountParams) error {
	accountAddress, err := a.prepareToAccount(params.ToAddress)
	if err != nil {
		return ierrors.Wrap(err, "failed to prepare account address")
	}

	// check the pool stake before delegating
	validatorResp, err := a.client.GetStaking(ctx, accountAddress)
	if err != nil {
		return ierrors.Wrap(err, "failed to get staking data from node")
	}

	poolStakeBefore := validatorResp.PoolStake
	a.LogInfof("Pool stake for validator %s before delegating: %d", accountAddress, poolStakeBefore)

	accData, faucetOutput, err := a.prepareFromAccount(ctx, params.FromAlias)
	if err != nil {
		return err
	}

	// get the latest block issuance data from the node
	congestionResp, issuerResp, version, err := a.RequestBlockIssuanceData(ctx, a.client, a.GenesisAccount)
	if err != nil {
		return ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}

	commitmentID := lo.Return1(issuerResp.LatestCommitment.ID())
	delegationOutput, issuingSlot, err := a.createDelegationOutput(params.Amount, accountAddress, accData, commitmentID)
	if err != nil {
		return ierrors.Wrap(err, "failed to create delegation output")
	}

	signedTx, err := a.createDelegationTransaction(faucetOutput, delegationOutput, commitmentID, issuingSlot)
	if err != nil {
		return ierrors.Wrap(err, "failed to build transaction")
	}

	// issue the transaction in a block
	blockID, err := a.PostWithBlock(ctx, a.client, signedTx, a.GenesisAccount, congestionResp, issuerResp, version)
	if err != nil {
		return ierrors.Wrapf(err, "failed to post block with ID %s", blockID)
	}

	a.LogInfof("Posted transaction: delegate %d IOTA tokens from account %s to validator %s", params.Amount, params.FromAlias, params.ToAddress)

	// wait for the delegation to start when the start epoch has been committed
	delegationStartSlot := a.client.LatestAPI().TimeProvider().EpochStart(delegationOutput.StartEpoch)
	a.LogInfof("Waiting for slot %d to be committed, when delegation starts", delegationStartSlot)
	if err := utils.AwaitCommitment(ctx, a.Logger, a.client, delegationStartSlot); err != nil {
		return ierrors.Wrap(err, "failed to await commitment of start epoch")
	}

	// check the pool stake after delegating
	validatorResp, err = a.client.GetStaking(ctx, accountAddress)
	if err != nil {
		return ierrors.Wrap(err, "failed to get staking data from node")
	}

	poolStakeAfter := validatorResp.PoolStake
	a.LogInfof("Pool stake for validator %s after delegating: %d", accountAddress, poolStakeAfter)

	if poolStakeAfter-poolStakeBefore != iotago.BaseToken(params.Amount) {
		return ierrors.Errorf("delegated amount %d was not correctly added to pool stake. Pool stake before: %d. Pool stake after %d.", params.Amount, poolStakeBefore, poolStakeAfter)
	}

	a.LogInfof("Delegation successful. Pool stake increased by %d", params.Amount)

	return nil
}

func (a *AccountWallet) prepareToAccount(toAddress string) (*iotago.AccountAddress, error) {
	//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	_, address, err := iotago.ParseBech32(toAddress)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to parse account address")
	}

	// nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	accountAddress := address.(*iotago.AccountAddress)

	return accountAddress, nil
}

func (a *AccountWallet) prepareFromAccount(ctx context.Context, fromAlias string) (*models.AccountData, *models.Output, error) {
	// get the account from which we will delegate tokens
	accData, err := a.GetAccount(fromAlias)
	if err != nil {
		return nil, nil, ierrors.Wrap(err, "failed to get account data for delegator account")
	}

	// get faucet funds for delegation output
	faucetOutput, err := a.getFaucetFundsOutput(ctx, iotago.AddressEd25519)
	if err != nil {
		return nil, nil, ierrors.Wrap(err, "failed to get faucet funds for delegation output")
	}

	return accData, faucetOutput, nil
}

func (a *AccountWallet) createDelegationOutput(amount iotago.BaseToken, accountAddress *iotago.AccountAddress, accData *models.AccountData, commitmentID iotago.CommitmentID) (*iotago.DelegationOutput, iotago.SlotIndex, error) {
	issuingTime := time.Now()
	api := a.client.LatestAPI()
	issuingSlot := api.TimeProvider().SlotFromTime(issuingTime)

	// create a delegation output
	delegationOutput, err := builder.NewDelegationOutputBuilder(accountAddress, accData.Account.OwnerAddress(), amount).
		DelegatedAmount(amount).
		StartEpoch(a.delegationStart(api, issuingSlot, commitmentID.Slot())).
		EndEpoch(a.delegationEnd(api, issuingSlot, commitmentID.Slot())).
		Build()
	if err != nil {
		return nil, 0, ierrors.Wrap(err, "failed to build delegation output")
	}

	minDeposit := lo.PanicOnErr(api.StorageScoreStructure().MinDeposit(delegationOutput))
	if delegationOutput.Amount < minDeposit {
		a.LogDebugf("Delegated amount does not cover the minimum storage deposit of %d", minDeposit)
	}
	a.LogDebugf("Created delegation output with delegated amount %d, start epoch %d and end epoch %d", delegationOutput.Amount, delegationOutput.StartEpoch, delegationOutput.EndEpoch)

	return delegationOutput, issuingSlot, nil
}

func (a *AccountWallet) createDelegationTransaction(faucetOutput *models.Output, delegationOutput *iotago.DelegationOutput, commitmentID iotago.CommitmentID, issuingSlot iotago.SlotIndex) (*iotago.SignedTransaction, error) {
	// create a transaction with the delegation output
	apiForSlot := a.client.APIForSlot(issuingSlot)
	txBuilder := builder.NewTransactionBuilder(apiForSlot)
	txBuilder.AddInput(&builder.TxInput{
		UnlockTarget: faucetOutput.Address,
		InputID:      faucetOutput.OutputID,
		Input:        faucetOutput.OutputStruct,
	})
	txBuilder.AddOutput(delegationOutput)
	txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: commitmentID})
	txBuilder.SetCreationSlot(issuingSlot)
	txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))

	addressSigner, err := a.GetAddrSignerForIndexes(faucetOutput)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get address signer")
	}

	return txBuilder.Build(addressSigner)
}

func (a *AccountWallet) delegationStart(apiForSlot iotago.API, issuingSlot iotago.SlotIndex, commitmentSlot iotago.SlotIndex) iotago.EpochIndex {
	pastBoundedSlotIndex := commitmentSlot + apiForSlot.ProtocolParameters().MaxCommittableAge()
	pastBoundedEpochIndex := apiForSlot.TimeProvider().EpochFromSlot(pastBoundedSlotIndex)

	registrationSlot := a.registrationSlot(apiForSlot, issuingSlot)

	if pastBoundedSlotIndex <= registrationSlot {
		return pastBoundedEpochIndex + 1
	}

	return pastBoundedEpochIndex + 2
}

func (a *AccountWallet) delegationEnd(apiForSlot iotago.API, issuingSlot iotago.SlotIndex, commitmentSlot iotago.SlotIndex) iotago.EpochIndex {
	futureBoundedSlotIndex := commitmentSlot + apiForSlot.ProtocolParameters().MinCommittableAge()
	futureBoundedEpochIndex := apiForSlot.TimeProvider().EpochFromSlot(futureBoundedSlotIndex)

	registrationSlot := a.registrationSlot(apiForSlot, issuingSlot)

	if futureBoundedEpochIndex <= iotago.EpochIndex(registrationSlot) {
		return futureBoundedEpochIndex
	}

	return futureBoundedEpochIndex + 1
}

func (a *AccountWallet) registrationSlot(apiForSlot iotago.API, slot iotago.SlotIndex) iotago.SlotIndex {
	return apiForSlot.TimeProvider().EpochEnd(apiForSlot.TimeProvider().EpochFromSlot(slot)) - apiForSlot.ProtocolParameters().EpochNearingThreshold()
}
