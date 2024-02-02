package walletmanager

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

func (m *Manager) delegateToAccount(ctx context.Context, params *DelegateAccountParams) error {
	m.LogInfo("Delegating to account...")
	wallet := m.getOrCreateWallet(params.FromAlias)
	accountAddress, err := m.prepareToAccount(params.ToAddress)
	if err != nil {
		return ierrors.Wrap(err, "failed to prepare account address")
	}

	// check the pool stake before delegating
	var poolStakeBefore, poolStakeAfter iotago.BaseToken
	if params.CheckPool {
		validatorResp, err := m.Client.GetStaking(ctx, accountAddress)
		if err != nil {
			return ierrors.Wrap(err, "failed to get staking data from node")
		}

		poolStakeBefore = validatorResp.PoolStake
		m.LogInfof("Pool stake for validator %s before delegating: %d", accountAddress, poolStakeBefore)
	}

	faucetOutputs, err := m.prepareInputs(ctx, m.Client, wallet, params)
	if err != nil {
		return err
	}

	// get the latest block issuance data from the node
	congestionResp, issuerResp, version, err := m.RequestBlockIssuanceData(ctx, m.Client, m.GenesisAccount())
	if err != nil {
		return ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}

	commitmentID := lo.Return1(issuerResp.LatestCommitment.ID())
	issuingSlot := m.Client.LatestAPI().TimeProvider().SlotFromTime(time.Now())

	signedTx, output, err := m.createDelegationTransaction(wallet, params, accountAddress, faucetOutputs, commitmentID, issuingSlot)
	if err != nil {
		return ierrors.Wrap(err, "failed to build transaction")
	}

	// issue the transaction in a block
	blockID, err := m.PostWithBlock(ctx, m.Client, signedTx, m.GenesisAccount(), congestionResp, issuerResp, version)
	if err != nil {
		return ierrors.Wrapf(err, "failed to post block with ID %s", blockID)
	}

	m.LogInfof("Posted transaction: delegate %d tokens from %s to validator %s", params.Amount, params.FromAlias, params.ToAddress)
	if err := utils.AwaitBlockAndPayloadAcceptance(ctx, m.Logger, m.Client, blockID); err != nil {
		return ierrors.Wrap(err, "failed to await block and payload acceptance")
	}
	// register the delegation output and its signing keys etc. in the wallet
	m.registerDelegationOutput(params.FromAlias, output)

	if delegations, err := m.GetDelegations(params.FromAlias); err != nil {
		m.LogInfof("No delegations for alias %s", params.FromAlias)
	} else {
		m.LogInfof("Delegations for alias %s:\n", params.FromAlias)
		for i, delegation := range delegations {
			// nolint:forcetypeassert // we know that the output is of type *iotago.DelegationOutput
			m.LogInfof("Delegation %d: %d tokens delegated to validator %s", i, delegation.Amount, delegation.DelegatedToBechAddress)
		}
	}

	if params.CheckPool {
		// wait for the delegation to start when the start epoch has been committed
		// nolint:forcetypeassert // we know that the output is of type *iotago.DelegationOutput
		delegationOutput := output.OutputStruct.(*iotago.DelegationOutput)
		delegationStartSlot := m.Client.LatestAPI().TimeProvider().EpochStart(delegationOutput.StartEpoch)
		m.LogInfof("Waiting for slot %d to be committed, when delegation starts", delegationStartSlot)
		if err := utils.AwaitCommitment(ctx, m.Logger, m.Client, delegationStartSlot); err != nil {
			return ierrors.Wrap(err, "failed to await commitment of start epoch")
		}

		// check the pool stake after delegating
		validatorResp, err := m.Client.GetStaking(ctx, accountAddress)
		if err != nil {
			return ierrors.Wrap(err, "failed to get staking data from node")
		}

		poolStakeAfter = validatorResp.PoolStake
		m.LogInfof("Pool stake for validator %s after delegating: %d", accountAddress, poolStakeAfter)

		if poolStakeAfter-poolStakeBefore != params.Amount {
			return ierrors.Errorf("delegated amount %d was not correctly added to pool stake. Pool stake before: %d. Pool stake after %d.", params.Amount, poolStakeBefore, poolStakeAfter)
		}

		m.LogInfof("Delegation successful. Pool stake increased by %d", params.Amount)
	}

	return nil
}

func (m *Manager) prepareToAccount(toAddress string) (*iotago.AccountAddress, error) {
	//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	_, address, err := iotago.ParseBech32(toAddress)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to parse account address")
	}

	// nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	accountAddress := address.(*iotago.AccountAddress)

	return accountAddress, nil
}

func (m *Manager) prepareInputs(ctx context.Context, clt models.Client, wallet *Wallet, params *DelegateAccountParams) ([]*models.OutputData, error) {
	if params.FromAlias == "" {
		params.FromAlias = GenesisAccountAlias
	}

	var inputs []*models.OutputData
	var totalInputAmount iotago.BaseToken
	// get faucet funds for delegation output
	for i := 0; i < iotago.MaxInputsCount; i++ {
		faucetOutput, err := m.getFaucetFundsOutput(ctx, clt, wallet, iotago.AddressEd25519)
		if err != nil {
			return nil, ierrors.Wrap(err, "failed to get faucet funds for delegation output")
		}
		inputs = append(inputs, faucetOutput)
		totalInputAmount += faucetOutput.OutputStruct.BaseTokenAmount()
		if totalInputAmount == params.Amount {
			return inputs, nil
		}
		// check if there is enough for the storage deposit for a remainder output
		minDeposit := lo.PanicOnErr(clt.LatestAPI().StorageScoreStructure().MinDeposit(faucetOutput.OutputStruct))
		if totalInputAmount >= params.Amount+minDeposit {
			return inputs, nil
		}
	}

	return nil, ierrors.New("failed to get enough faucet funds for delegation output")
}

func (m *Manager) createDelegationOutputs(wallet *Wallet, inputAmount iotago.BaseToken, delegatedAmount iotago.BaseToken, issuingSlot iotago.SlotIndex, accountAddress *iotago.AccountAddress, commitmentID iotago.CommitmentID) ([]*models.OutputData, error) {
	var outputs []*models.OutputData
	api := m.Client.APIForSlot(issuingSlot)
	// get the address and private key for the delegator alias
	ownerAddress, privateKey, index := wallet.getAddress(iotago.AddressEd25519)

	// create a delegation output
	delegationOutput, err := builder.NewDelegationOutputBuilder(accountAddress, ownerAddress, delegatedAmount).
		DelegatedAmount(delegatedAmount).
		StartEpoch(m.delegationStart(api, issuingSlot, commitmentID.Slot())).
		Build()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to build delegation output")
	}

	minDeposit := lo.PanicOnErr(api.StorageScoreStructure().MinDeposit(delegationOutput))
	if delegationOutput.Amount < minDeposit {
		m.LogDebugf("Delegated amount does not cover the minimum storage deposit of %d", minDeposit)
	}
	delegationModelOutput, err := models.NewOutputDataWithEmptyID(api, ownerAddress, index, privateKey, delegationOutput)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to create delegation output")
	}
	outputs = append(outputs, delegationModelOutput)
	m.LogDebugf("Created delegation output with delegated amount %d, start epoch %d and end epoch %d", delegationOutput.Amount, delegationOutput.StartEpoch, delegationOutput.EndEpoch)

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
		m.LogDebugf("Created remainder basic output with amount %d", remainder)
	}

	return outputs, nil
}

func (m *Manager) createDelegationTransaction(wallet *Wallet, params *DelegateAccountParams, toAccountAddress *iotago.AccountAddress, inputs []*models.OutputData, commitmentID iotago.CommitmentID, issuingSlot iotago.SlotIndex) (*iotago.SignedTransaction, *models.OutputData, error) {
	// create a transaction with the delegation output
	apiForSlot := m.Client.APIForSlot(issuingSlot)
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
	outputs, err := m.createDelegationOutputs(wallet, totalInputAmount, params.Amount, issuingSlot, toAccountAddress, commitmentID)
	if err != nil {
		return nil, nil, ierrors.Wrap(err, "failed to create delegation output")
	}
	for _, output := range outputs {
		txBuilder.AddOutput(output.OutputStruct)
	}
	txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: commitmentID})
	txBuilder.SetCreationSlot(issuingSlot)
	txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))

	addressSigner, err := wallet.GetAddrSignerForIndexes(inputs...)
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

func (m *Manager) delegationStart(apiForSlot iotago.API, issuingSlot iotago.SlotIndex, commitmentSlot iotago.SlotIndex) iotago.EpochIndex {
	pastBoundedSlotIndex := commitmentSlot + apiForSlot.ProtocolParameters().MaxCommittableAge()
	pastBoundedEpochIndex := apiForSlot.TimeProvider().EpochFromSlot(pastBoundedSlotIndex)

	registrationSlot := utils.DelegationRegistrationSlot(apiForSlot, issuingSlot)

	if pastBoundedSlotIndex <= registrationSlot {
		return pastBoundedEpochIndex + 1
	}

	return pastBoundedEpochIndex + 2
}
