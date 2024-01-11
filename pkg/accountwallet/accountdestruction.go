package accountwallet

import (
	"context"
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/wallet"
)

func (a *AccountWallet) destroyAccount(ctx context.Context, alias string) error {
	accData, err := a.GetAccount(alias)
	if err != nil {
		return err
	}
	// get output from node
	// From TIP42: Indexers and node plugins shall map the account address of the output derived with Account ID to the regular address -> output mapping table, so that given an Account Address, its most recent unspent account output can be retrieved.
	// TODO: use correct outputID
	accountOutput := a.client.GetOutput(ctx, accData.OutputID)
	switch accountOutput.Type() {
	case iotago.OutputBasic:
		a.LogInfof("Cannot destroy implicit account %s", alias)

		return nil
	}

	keyManager, err := wallet.NewKeyManager(a.seed[:], BIP32PathForIndex(accData.Index))
	if err != nil {
		return err
	}
	{
		// first, transition the account so block issuer feature expires if it is not already.
		issuingTime := time.Now()
		issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
		apiForSlot := a.client.APIForSlot(issuingSlot)
		// get the latest block issuance data from the node
		congestionResp, issuerResp, version, err := a.RequestBlockIssuanceData(ctx, a.client, a.GenesisAccount)
		if err != nil {
			return ierrors.Wrap(err, "failed to request block built data for the faucet account")
		}
		commitmentID := lo.Return1(issuerResp.LatestCommitment.ID())
		commitmentSlot := commitmentID.Slot()
		pastBoundedSlot := commitmentSlot + apiForSlot.ProtocolParameters().MaxCommittableAge()
		// transition it to expire if it is not already as soon as possible
		if accountOutput.FeatureSet().BlockIssuer().ExpirySlot > pastBoundedSlot {
			// start building the transaction
			txBuilder := builder.NewTransactionBuilder(apiForSlot)
			// add the account output as input
			txBuilder.AddInput(&builder.TxInput{
				UnlockTarget: accountOutput.UnlockConditionSet().Address().Address,
				InputID:      accData.OutputID,
				Input:        accountOutput,
			})
			// create an account output with updated expiry slot set to commitment slot + MaxCommittableAge (pastBoundedSlot)
			// nolint:forcetypeassert // we know that this is an account output
			accountBuilder := builder.NewAccountOutputBuilderFromPrevious(accountOutput.(*iotago.AccountOutput))
			accountBuilder.BlockIssuer(accountOutput.FeatureSet().BlockIssuer().BlockIssuerKeys, pastBoundedSlot)
			expiredAccountOutput := accountBuilder.MustBuild()
			// add the expired account output as output
			txBuilder.AddOutput(expiredAccountOutput)
			// add the commitment input
			txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: commitmentID})
			// add a block issuance credit input
			txBuilder.AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: accData.Account.ID()})
			// set the creation slot to the issuance slot
			txBuilder.SetCreationSlot(issuingSlot)
			// set the transaction capabilities to be able to do anything
			txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
			// build the transaction
			signedTx, err := txBuilder.Build(keyManager.AddressSigner())
			if err != nil {
				return ierrors.Wrap(err, "failed to build transaction")
			}

			// issue the transaction in a block
			blockID, err := a.PostWithBlock(ctx, a.client, signedTx, a.GenesisAccount, congestionResp, issuerResp, version)
			if err != nil {
				return ierrors.Wrapf(err, "failed to post block with ID %s", blockID)
			}
			a.LogInfof("Posted transaction: transition account to expire in slot %d\nBech addr: %s", pastBoundedSlot, accData.Account.Address().Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))

			// check the status of the transaction
			expiredAccountOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
			err = a.checkAccountStatus(ctx, blockID, lo.PanicOnErr(signedTx.Transaction.ID()), expiredAccountOutputID, accData.Account.Address())
			if err != nil {
				return ierrors.Wrap(err, "failure checking for commitment of account transition")
			}

			// update the account output details in the wallet
			a.registerAccount(alias, accData.Account.ID(), expiredAccountOutputID, accData.Index, accData.Account.PrivateKey())
		}
		// wait until the expiry time has passed
		if time.Now().Before(apiForSlot.TimeProvider().SlotEndTime(pastBoundedSlot)) {
			a.LogInfof("Waiting for slot %d when account expires", pastBoundedSlot)
			time.Sleep(time.Until(apiForSlot.TimeProvider().SlotEndTime(pastBoundedSlot)))
		}
	}
	{
		// next, issue a transaction to destroy the account output
		issuingTime := time.Now()
		issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
		apiForSlot := a.client.APIForSlot(issuingSlot)
		// get the latest block issuance data from the node
		congestionResp, issuerResp, version, err := a.RequestBlockIssuanceData(ctx, a.client, a.GenesisAccount)
		if err != nil {
			return ierrors.Wrap(err, "failed to request block built data for the faucet account")
		}
		commitmentID := lo.Return1(issuerResp.LatestCommitment.ID())
		// start building the transaction
		txBuilder := builder.NewTransactionBuilder(apiForSlot)
		// add the expired account output on the input side
		accData, err := a.GetAccount(alias)
		if err != nil {
			return err
		}
		expiredAccountOutput := a.client.GetOutput(ctx, accData.OutputID)
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: expiredAccountOutput.UnlockConditionSet().Address().Address,
			InputID:      accData.OutputID,
			Input:        expiredAccountOutput,
		})
		// add a basic output to output side
		addr, _, _ := a.getAddress(iotago.AddressEd25519)
		basicOutput := builder.NewBasicOutputBuilder(addr, expiredAccountOutput.BaseTokenAmount()).MustBuild()
		txBuilder.AddOutput(basicOutput)
		// set the creation slot to the issuance slot
		txBuilder.SetCreationSlot(issuingSlot)
		// add the commitment input
		txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: commitmentID})
		// add a block issuance credit input
		txBuilder.AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: accData.Account.ID()})
		// set the transaction capabilities to be able to do anything
		txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
		// build the transaction
		signedTx, err := txBuilder.Build(keyManager.AddressSigner())
		if err != nil {
			return ierrors.Wrap(err, "failed to build transaction")
		}

		// DEBUG: check that BIC is not negative
		conRespAccount, _, _, err := a.RequestBlockIssuanceData(ctx, a.client, accData.Account)
		if err != nil {
			return ierrors.Wrap(err, "failed to get block issuance data for account")
		}
		a.LogInfo("BIC: ", conRespAccount.BlockIssuanceCredits, ", Expiry: ", expiredAccountOutput.FeatureSet().BlockIssuer().ExpirySlot, ", Current slot: ", issuingSlot)

		// issue the transaction in a block
		blockID, err := a.PostWithBlock(ctx, a.client, signedTx, a.GenesisAccount, congestionResp, issuerResp, version)
		if err != nil {
			return ierrors.Wrapf(err, "failed to post block with ID %s", blockID)
		}
		a.LogInfof("Posted transaction: destroy account\nBech addr: %s", accData.Account.Address().Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))

		// check the status of the transaction
		basicOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
		err = a.checkAccountStatus(ctx, blockID, lo.PanicOnErr(signedTx.Transaction.ID()), basicOutputID, nil)
		if err != nil {
			return ierrors.Wrap(err, "failure checking for commitment of account transition")
		}

		// check that the basic output is retrievable
		// TODO: move this to checkIndexer within the checkAccountStatus function
		// check for the basic output being committed, indicating the account output has been consumed (destroyed)
		if output := a.client.GetOutput(ctx, basicOutputID); output == nil {
			return ierrors.Wrap(err, "failed to get basic output from node after commitment")
		}

		// remove account from wallet
		delete(a.accountsAliases, alias)

		a.LogInfof("Account %s has been destroyed", alias)
	}

	return nil
}
