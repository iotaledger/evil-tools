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

func (a *AccountWallets) delegateToAccount(ctx context.Context, params *DelegateAccountParams) error {
	wallet := a.GetOrCreateWallet(params.FromAlias)
	accountAddress, err := wallet.prepareToAccount(params.ToAddress)
	if err != nil {
		return ierrors.Wrap(err, "failed to prepare account address")
	}

	// check the pool stake before delegating
	var poolStakeBefore, poolStakeAfter iotago.BaseToken
	if params.CheckPool {
		validatorResp, err := a.client.GetStaking(ctx, accountAddress)
		if err != nil {
			return ierrors.Wrap(err, "failed to get staking data from node")
		}

		poolStakeBefore := validatorResp.PoolStake
		a.LogInfof("Pool stake for validator %s before delegating: %d", accountAddress, poolStakeBefore)
	}

	faucetOutputs, err := wallet.prepareInputs(ctx, params)
	if err != nil {
		return err
	}

	// get the latest block issuance data from the node
	congestionResp, issuerResp, version, err := wallet.RequestBlockIssuanceData(ctx, a.client, a.GenesisAccount)
	if err != nil {
		return ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}

	commitmentID := lo.Return1(issuerResp.LatestCommitment.ID())
	issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(time.Now())

	signedTx, output, err := wallet.createDelegationTransaction(params, accountAddress, faucetOutputs, commitmentID, issuingSlot)
	if err != nil {
		return ierrors.Wrap(err, "failed to build transaction")
	}

	// issue the transaction in a block
	blockID, err := wallet.PostWithBlock(ctx, a.client, signedTx, a.GenesisAccount, congestionResp, issuerResp, version)
	if err != nil {
		return ierrors.Wrapf(err, "failed to post block with ID %s", blockID)
	}

	a.LogInfof("Posted transaction: delegate %d tokens from %s to validator %s", params.Amount, params.FromAlias, params.ToAddress)
	if err := utils.AwaitBlockAndPayloadAcceptance(ctx, a.Logger, a.client, blockID); err != nil {
		return ierrors.Wrap(err, "failed to await block and payload acceptance")
	}
	// register the delegation output and its signing keys etc. in the wallet
	a.registerOutput(params.FromAlias, output)

	if delegations, err := a.GetDelegations(params.FromAlias); err != nil {
		a.LogInfof("No delegations for alias %s", params.FromAlias)
	} else {
		a.LogInfof("Delegations for alias %s:\n", params.FromAlias)
		for i, delegation := range delegations {
			// nolint:forcetypeassert // we know that the output is of type *iotago.DelegationOutput
			delegationOutput := delegation.OutputStruct.(*iotago.DelegationOutput)
			a.LogInfof("Delegation %d: %d tokens delegated to validator %s", i, delegationOutput.DelegatedAmount, delegationOutput.ValidatorAddress.Bech32(a.API.ProtocolParameters().Bech32HRP()))
		}
	}

	if params.CheckPool {
		// wait for the delegation to start when the start epoch has been committed
		// nolint:forcetypeassert // we know that the output is of type *iotago.DelegationOutput
		delegationOutput := output.OutputStruct.(*iotago.DelegationOutput)
		delegationStartSlot := a.client.LatestAPI().TimeProvider().EpochStart(delegationOutput.StartEpoch)
		a.LogInfof("Waiting for slot %d to be committed, when delegation starts", delegationStartSlot)
		if err := utils.AwaitCommitment(ctx, a.Logger, a.client, delegationStartSlot); err != nil {
			return ierrors.Wrap(err, "failed to await commitment of start epoch")
		}

		// check the pool stake after delegating
		validatorResp, err := a.client.GetStaking(ctx, accountAddress)
		if err != nil {
			return ierrors.Wrap(err, "failed to get staking data from node")
		}

		poolStakeAfter = validatorResp.PoolStake
		a.LogInfof("Pool stake for validator %s after delegating: %d", accountAddress, poolStakeAfter)

		if poolStakeAfter-poolStakeBefore != params.Amount {
			return ierrors.Errorf("delegated amount %d was not correctly added to pool stake. Pool stake before: %d. Pool stake after %d.", params.Amount, poolStakeBefore, poolStakeAfter)
		}

		a.LogInfof("Delegation successful. Pool stake increased by %d", params.Amount)
	}

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

func (a *AccountWallet) prepareInputs(ctx context.Context, params *DelegateAccountParams) ([]*models.OutputData, error) {
	if params.FromAlias == "" {
		params.FromAlias = GenesisAccountAlias
	}

	var inputs []*models.OutputData
	var totalInputAmount iotago.BaseToken
	// get faucet funds for delegation output
	for i := 0; i < iotago.MaxInputsCount; i++ {
		faucetOutput, err := a.getFaucetFundsOutput(ctx, iotago.AddressEd25519)
		if err != nil {
			return nil, ierrors.Wrap(err, "failed to get faucet funds for delegation output")
		}
		inputs = append(inputs, faucetOutput)
		totalInputAmount += faucetOutput.OutputStruct.BaseTokenAmount()
		if totalInputAmount == params.Amount {
			return inputs, nil
		}
		// check if there is enough for the storage deposit for a remainder output
		minDeposit := lo.PanicOnErr(a.API.StorageScoreStructure().MinDeposit(faucetOutput.OutputStruct))
		if totalInputAmount >= params.Amount+minDeposit {
			return inputs, nil
		}
	}

	return nil, ierrors.New("failed to get enough faucet funds for delegation output")
}

func (a *AccountWallet) createDelegationOutputs(inputAmount iotago.BaseToken, delegatedAmount iotago.BaseToken, issuingSlot iotago.SlotIndex, accountAddress *iotago.AccountAddress, commitmentID iotago.CommitmentID) ([]*models.OutputData, error) {
	var outputs []*models.OutputData
	api := a.client.APIForSlot(issuingSlot)
	// get the address and private key for the delegator alias
	ownerAddress, privateKey, index := a.getAddress(iotago.AddressEd25519)

	// create a delegation output
	delegationOutput, err := builder.NewDelegationOutputBuilder(accountAddress, ownerAddress, delegatedAmount).
		DelegatedAmount(delegatedAmount).
		StartEpoch(a.delegationStart(api, issuingSlot, commitmentID.Slot())).
		EndEpoch(a.delegationEnd(api, issuingSlot, commitmentID.Slot())).
		Build()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to build delegation output")
	}

	minDeposit := lo.PanicOnErr(api.StorageScoreStructure().MinDeposit(delegationOutput))
	if delegationOutput.Amount < minDeposit {
		a.LogDebugf("Delegated amount does not cover the minimum storage deposit of %d", minDeposit)
	}
	delegationModelOutput, err := models.NewOutputDataWithEmptyID(api, ownerAddress, index, privateKey, delegationOutput)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to create delegation output")
	}
	outputs = append(outputs, delegationModelOutput)
	a.LogDebugf("Created delegation output with delegated amount %d, start epoch %d and end epoch %d", delegationOutput.Amount, delegationOutput.StartEpoch, delegationOutput.EndEpoch)

	// create a remainder for any remaining faucet funds
	remainder := inputAmount - delegatedAmount
	if remainder > 0 {
		remainderOutput, err := builder.NewBasicOutputBuilder(ownerAddress, remainder).Build()
		if err != nil {
			return nil, ierrors.Wrap(err, "failed to build remainder output")
		}
		remainderModelOutput, err := models.NewOutputDataWithEmptyID(api, ownerAddress, index, privateKey, remainderOutput)
		if err != nil {
			return nil, ierrors.Wrap(err, "failed to create model output for remainder output")
		}
		outputs = append(outputs, remainderModelOutput)
		a.LogDebugf("Created remainder basic output with amount %d", remainder)
	}

	return outputs, nil
}

func (a *AccountWallet) createDelegationTransaction(params *DelegateAccountParams, toAccountAddress *iotago.AccountAddress, inputs []*models.OutputData, commitmentID iotago.CommitmentID, issuingSlot iotago.SlotIndex) (*iotago.SignedTransaction, *models.OutputData, error) {
	// create a transaction with the delegation output
	apiForSlot := a.client.APIForSlot(issuingSlot)
	var totalInputAmount iotago.BaseToken
	txBuilder := builder.NewTransactionBuilder(apiForSlot)
	for _, input := range inputs {
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: input.Address,
			InputID:      input.OutputID,
			Input:        input.OutputStruct,
		})
		totalInputAmount += input.OutputStruct.BaseTokenAmount()
	}
	outputs, err := a.createDelegationOutputs(totalInputAmount, params.Amount, issuingSlot, toAccountAddress, commitmentID)
	if err != nil {
		return nil, nil, ierrors.Wrap(err, "failed to create delegation output")
	}
	for _, output := range outputs {
		txBuilder.AddOutput(output.OutputStruct)
	}
	txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: commitmentID})
	txBuilder.SetCreationSlot(issuingSlot)
	txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))

	addressSigner, err := a.GetAddrSignerForIndexes(inputs...)
	if err != nil {
		return nil, nil, ierrors.Wrap(err, "failed to get address signer")
	}

	signedTx, err := txBuilder.Build(addressSigner)
	if err != nil {
		return nil, nil, ierrors.Wrap(err, "failed to build tx")
	}

	delegationOutput := outputs[0]
	delegationOutput.OutputID = iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)

	return signedTx, delegationOutput, nil

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
