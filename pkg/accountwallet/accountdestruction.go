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
	iotagowallet "github.com/iotaledger/iota.go/v4/wallet"
)

func (a *AccountWallets) destroyAccount(ctx context.Context, alias string) error {
	wallet := a.GetOrCreateWallet(alias)

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

	keyManager, err := iotagowallet.NewKeyManager(wallet.seed[:], BIP32PathForIndex(accData.Index))
	if err != nil {
		return err
	}
	{
		// first, transition the account so block issuer feature expires if it is not already.
		issuingTime := time.Now()
		issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
		apiForSlot := a.client.APIForSlot(issuingSlot)
		// get the latest block issuance data from the node
		congestionResp, issuerResp, version, err := wallet.RequestBlockIssuanceData(ctx, a.client, a.GenesisAccount)
		if err != nil {
			return ierrors.Wrap(err, "failed to request block built data for the faucet account")
		}
		commitmentID := lo.Return1(issuerResp.LatestCommitment.ID())
		commitmentSlot := commitmentID.Slot()
		// transition it to expire if it is not already expired relative to latest commitment
		if accountOutput.FeatureSet().BlockIssuer().ExpirySlot > commitmentSlot {
			pastBoundedSlot := commitmentSlot + apiForSlot.ProtocolParameters().MaxCommittableAge()
			// change the expiry slot to expire as soon as possible
			signedTx, err := wallet.changeExpirySlotTransaction(ctx, pastBoundedSlot, issuingSlot, accData, commitmentID, keyManager.AddressSigner())
			if err != nil {
				return ierrors.Wrap(err, "failed to build transaction")
			}
			// issue the transaction in a block
			blockID, err := wallet.PostWithBlock(ctx, a.client, signedTx, a.GenesisAccount, congestionResp, issuerResp, version)
			if err != nil {
				return ierrors.Wrapf(err, "failed to post block with ID %s", blockID)
			}
			a.LogInfof("Posted transaction: transition account to expire in slot %d\nBech addr: %s", pastBoundedSlot, accData.Account.Address().Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))

			// check the status of the transaction
			expiredAccountOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
			err = wallet.checkOutputStatus(ctx, blockID, lo.PanicOnErr(signedTx.Transaction.ID()), expiredAccountOutputID, accData.Account.Address())
			if err != nil {
				return ierrors.Wrap(err, "failure checking for commitment of account transition")
			}

			// update the account output details in the wallet
			a.registerAccount(alias, accData.Account.ID(), expiredAccountOutputID, accData.Index, accData.Account.PrivateKey())

			// wait until the expiry slot has been committed
			a.LogInfof("Waiting for expiry slot %d to be committed, 1 slot after expiry slot", pastBoundedSlot+1)
			if err := utils.AwaitCommitment(ctx, a.Logger, a.client, pastBoundedSlot+1); err != nil {
				return ierrors.Wrap(err, "failed to await commitment of expiry slot")
			}
		}

	}
	{
		// next, issue a transaction to destroy the account output
		issuingTime := time.Now()
		issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)

		// get the details of the expired account output
		accData, err := a.GetAccount(alias)
		if err != nil {
			return err
		}
		// get the latest block issuance data from the node
		congestionResp, issuerResp, version, err := wallet.RequestBlockIssuanceData(ctx, a.client, a.GenesisAccount)
		if err != nil {
			return ierrors.Wrap(err, "failed to request block built data for the faucet account")
		}
		commitmentID := lo.Return1(issuerResp.LatestCommitment.ID())

		// create a transaction destroying the account
		signedTx, err := wallet.destroyAccountTransaction(ctx, issuingSlot, accData, commitmentID, keyManager.AddressSigner())
		if err != nil {
			return ierrors.Wrap(err, "failed to build transaction")
		}
		// issue the transaction in a block
		blockID, err := wallet.PostWithBlock(ctx, a.client, signedTx, a.GenesisAccount, congestionResp, issuerResp, version)
		if err != nil {
			return ierrors.Wrapf(err, "failed to post block with ID %s", blockID)
		}
		a.LogInfof("Posted transaction: destroy account\nBech addr: %s", accData.Account.Address().Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))

		// check the status of the transaction
		basicOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
		err = wallet.checkOutputStatus(ctx, blockID, lo.PanicOnErr(signedTx.Transaction.ID()), basicOutputID, nil)
		if err != nil {
			return ierrors.Wrap(err, "failure checking for commitment of account transition")
		}

		// remove account from wallet
		a.deleteAccount(alias)

		a.LogInfof("Account %s has been destroyed", alias)
	}

	return nil
}

func (a *AccountWallet) changeExpirySlotTransaction(ctx context.Context, newExpirySlot iotago.SlotIndex, issuingSlot iotago.SlotIndex, accData *models.AccountData, commitmentID iotago.CommitmentID, addressSigner iotago.AddressSigner) (*iotago.SignedTransaction, error) {
	// start building the transaction
	apiForSlot := a.client.APIForSlot(issuingSlot)
	txBuilder := builder.NewTransactionBuilder(apiForSlot)
	accountOutput := a.client.GetOutput(ctx, accData.OutputID)

	// add the account output as input
	txBuilder.AddInput(&builder.TxInput{
		UnlockTarget: accountOutput.UnlockConditionSet().Address().Address,
		InputID:      accData.OutputID,
		Input:        accountOutput,
	})
	// create an account output with updated expiry slot set to commitment slot + MaxCommittableAge (pastBoundedSlot)
	// nolint:forcetypeassert // we know that this is an account output
	accountBuilder := builder.NewAccountOutputBuilderFromPrevious(accountOutput.(*iotago.AccountOutput))
	accountBuilder.BlockIssuer(accountOutput.FeatureSet().BlockIssuer().BlockIssuerKeys, newExpirySlot)
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
	return txBuilder.Build(addressSigner)
}

func (a *AccountWallet) destroyAccountTransaction(ctx context.Context, issuingSlot iotago.SlotIndex, accData *models.AccountData, commitmentID iotago.CommitmentID, addressSigner iotago.AddressSigner) (*iotago.SignedTransaction, error) {
	// start building the transaction
	apiForSlot := a.client.APIForSlot(issuingSlot)
	txBuilder := builder.NewTransactionBuilder(apiForSlot)
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
	return txBuilder.Build(addressSigner)
}
