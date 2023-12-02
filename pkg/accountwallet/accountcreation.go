package accountwallet

import (
	"context"
	"crypto/ed25519"
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

// createAccountImplicitly creates an implicit account by sending a faucet request for ImplicitAddressCreation funds.
func (a *AccountWallet) createAccountImplicitly(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	log.Debug("Creating an implicit account")
	// An implicit account has an implicitly defined Block Issuer Key, corresponding to the address itself.
	// Thus, implicit accounts can issue blocks by signing them with the private key corresponding to the public key
	// from which the Implicit Account Creation Address was derived.
	implicitAccountOutput, err := a.getModelFunds(ctx, iotago.AddressImplicitAccountCreation)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "Failed to create account")
	}

	implicitOutputID := implicitAccountOutput.OutputID
	accID := iotago.AccountIDFromOutputID(implicitOutputID)
	accountAddress, ok := accID.ToAddress().(*iotago.AccountAddress)
	if !ok {
		return iotago.EmptyAccountID, ierrors.New("failed to convert account id to address")
	}
	log.Debug("created implicit account output with outputID: ", implicitOutputID.ToHex(), " accountAddress: ", accountAddress.Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))

	err = a.checkAccountStatus(ctx, iotago.EmptyBlockID, implicitAccountOutput.OutputID.TransactionID(), implicitOutputID, accountAddress, accID)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}
	a.registerAccount(params.Alias, accID, implicitOutputID, implicitAccountOutput.AddressIndex, implicitAccountOutput.PrivateKey)

	if !params.Transition {
		log.Infof("Implicit account created, outputID: %s, implicit accountID: %s", implicitAccountOutput.OutputID.ToHex(), accountAddress.Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))

		return accID, nil
	}

	return a.transitionImplicitAccount(ctx, implicitAccountOutput, params)
}

func (a *AccountWallet) transitionImplicitAccount(ctx context.Context, implicitAccountOutput *models.Output, params *CreateAccountParams) (iotago.AccountID, error) {
	log.Debugf("Transitioning implicit account with implicitAccountID %s for alias %s to regular account", iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID).ToHex(), params.Alias)

	tokenBalance := implicitAccountOutput.OutputStruct.BaseTokenAmount() // + utils.SumOutputsBalance(additionalBasicInputs)
	accountID := iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID)
	//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	accountAddress := accountID.ToAddress().(*iotago.AccountAddress)

	// transition from implicit to regular account
	// TODO: maybe we cannot use the same blockissuer keys as implicit account, need to test with whole new keys
	accEd25519Addr, accPrivateKey, accAddrIndex := a.getAddress(iotago.AddressEd25519)
	accBlockIssuerKey := iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(accPrivateKey.Public().(ed25519.PublicKey))
	log.Infof("Address generated for account: %s", accEd25519Addr)
	accountOutput := builder.NewAccountOutputBuilder(accEd25519Addr, tokenBalance).
		AccountID(accountID).
		BlockIssuer(iotago.NewBlockIssuerKeys(accBlockIssuerKey), iotago.MaxSlotIndex).MustBuild()

	log.Infof("Created account %s with %d tokens\n", accountOutput.AccountID.ToHex(), accountOutput.Amount)
	implicitAccountHandler := wallet.NewEd25519Account(accountID, implicitAccountOutput.PrivateKey)

	// transaction preparation
	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(ctx, a.client, implicitAccountHandler)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request block built data for the implicit account")
	}

	inputs := []*models.Output{implicitAccountOutput}
	signedTx, err := a.createAccountCreationTransaction(inputs, accountOutput, congestionResp, issuerResp)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to create account creation transaction")
	}

	log.Debugf("Transition transaction created:\n%s\n", utils.SprintTransaction(a.client.LatestAPI(), signedTx))

	// post block with faucet
	blkID, err := a.PostWithBlock(ctx, a.client, signedTx, implicitAccountHandler, congestionResp, issuerResp, version)
	if err != nil {
		log.Errorf("Failed to post account with block: %s", err)

		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to post transaction")
	}
	log.Debugf("Block sent with ID: %s, and no error", blkID.ToHex())

	// get OutputID of account output
	accOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)

	//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	err = a.checkAccountStatus(ctx, blkID, lo.PanicOnErr(signedTx.Transaction.ID()), accOutputID, accountAddress, accountID)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}

	a.registerAccount(params.Alias, accountID, accOutputID, accAddrIndex, accPrivateKey)

	return accountID, nil
}

func createBlockIssuerKeysForImplicitAccountTransition(implicitAccountOutput *models.Output) iotago.BlockIssuerKeys {
	pubKey, isEd25519 := implicitAccountOutput.PrivateKey.Public().(ed25519.PublicKey)
	if !isEd25519 {
		return nil
	}

	implicitAccAddr := iotago.ImplicitAccountCreationAddressFromPubKey(pubKey)
	implicitBlockIssuerKey := iotago.Ed25519PublicKeyHashBlockIssuerKeyFromImplicitAccountCreationAddress(implicitAccAddr)
	blockIssuerKeys := iotago.NewBlockIssuerKeys(implicitBlockIssuerKey)

	return blockIssuerKeys
}

func (a *AccountWallet) createAccountWithFaucet(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	log.Debug("Creating an account with faucet funds and mana")
	// request enough funds to cover the mana required for account creation
	creationOutput, err := a.getModelFunds(ctx, iotago.AddressEd25519)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request enough funds for account creation")
	}
	accAddr, accPrivateKey, accAddrIndex := a.getAddress(iotago.AddressEd25519)

	blockIssuerKeys, err := a.getAccountPublicKeys(accPrivateKey.Public())
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to get account address and keys")
	}
	accountOutput := builder.NewAccountOutputBuilder(accAddr, creationOutput.OutputStruct.BaseTokenAmount()).
		// mana  will be updated after allotment
		// no accountID should be specified during the account creation
		BlockIssuer(blockIssuerKeys, iotago.MaxSlotIndex).MustBuild()

	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(ctx, a.client, a.faucet.account)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}

	signedTx, err := a.createAccountCreationTransaction([]*models.Output{creationOutput}, accountOutput, congestionResp, issuerResp)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to create account transaction")
	}
	log.Debugf("Transaction for account creation signed: %s\n", utils.SprintTransaction(a.client.LatestAPI(), signedTx))

	blkID, err := a.PostWithBlock(ctx, a.client, signedTx, a.faucet.account, congestionResp, issuerResp, version)
	if err != nil {
		log.Errorf("Failed to post account with block: %s", err)

		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to post transaction")
	}
	log.Debugf("Account creation transaction posted with block: %s, in slot: %d with no error.\n", blkID.ToHex(), blkID.Slot())
	//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress

	accountOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
	accountID := iotago.AccountIDFromOutputID(accountOutputID)
	//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	accountAddress := accountID.ToAddress().(*iotago.AccountAddress)
	err = a.checkAccountStatus(ctx, blkID, lo.PanicOnErr(signedTx.Transaction.ID()), accountOutputID, accountAddress, accountID)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}

	a.registerAccount(params.Alias, accountID, accountOutputID, accAddrIndex, accPrivateKey)

	return accountID, nil
}

func (a *AccountWallet) createAccountCreationTransaction(inputs []*models.Output, accountOutput *iotago.AccountOutput, congestionResp *api.CongestionResponse, issuerResp *api.IssuanceBlockHeaderResponse) (*iotago.SignedTransaction, error) {
	currentTime := time.Now()
	currentSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(currentTime)
	apiForSlot := a.client.APIForSlot(currentSlot)

	txBuilder := builder.NewTransactionBuilder(apiForSlot)
	for _, output := range inputs {
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: output.Address,
			InputID:      output.OutputID,
			Input:        output.OutputStruct,
		})
	}

	txBuilder.AddOutput(accountOutput)
	txBuilder.SetCreationSlot(currentSlot)

	commitmentID, _ := issuerResp.LatestCommitment.ID()
	txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: commitmentID})
	txBuilder.AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: accountOutput.AccountID})
	txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
	// allot required mana to the implicit account
	a.logMissingMana(txBuilder, congestionResp.ReferenceManaCost, a.faucet.account)
	txBuilder.AllotAllMana(txBuilder.CreationSlot(), accountOutput.AccountID)

	// sign the transaction
	addrSigner, err := a.GetAddrSignerForIndexes(inputs...)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get address signer")
	}

	signedTx, err := txBuilder.Build(addrSigner)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to build tx")
	}

	return signedTx, nil
}
