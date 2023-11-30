package accountwallet

import (
	"context"
	"crypto/ed25519"
	"fmt"
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

// createImplicitAccount creates an implicit account by creating a tx with implicit outputs, it avoids faucet, as it only requests funds for the input on the transaction.
func (a *AccountWallet) createImplicitAccount(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	log.Debug("Creating an implicit account")
	// TODO fix RequestManaAndFundsFromTheFaucet and use it to always get enough storage deposit for the account output
	//minimumTokenAmount := utils.MinIssuerAccountAmount(a.client.CommittedAPI().ProtocolParameters())

	//inputs, err := a.RequestManaAndFundsFromTheFaucet(ctx, 0, minimumTokenAmount)
	//if err != nil {
	//	return ierrors.Wrap(err, "failed to request enough funds for account creation")
	//}

	out, err := a.getModelFunds(ctx, iotago.AddressEd25519)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to get funds")
	}
	inputs := []*models.Output{out}
	balance := utils.SumOutputsBalance(inputs)
	implicitAddr, privKey, addrIndex := a.getAddress(iotago.AddressImplicitAccountCreation)

	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(ctx, a.client, a.faucet.account)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request block built data")
	}
	signedTx, err := a.createImplicitOutputTransaction(inputs, implicitAddr, balance, congestionResp.ReferenceManaCost, lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to create implicit output transaction")
	}
	log.Debugf(utils.SprintTransaction(signedTx))

	// issue with block
	blkID, err := a.PostWithBlock(ctx, a.client, signedTx, a.faucet.account, congestionResp, issuerResp, version)
	if err != nil {
		log.Errorf("Failed to post account with block: %s", err)

		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to post transaction")
	}

	txID, err := signedTx.Transaction.ID()
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to get tx id")
	}
	implicitOutputID := iotago.OutputIDFromTransactionIDAndIndex(txID, 0)
	accID := iotago.AccountIDFromOutputID(implicitOutputID)
	accountAddress, ok := accID.ToAddress().(*iotago.AccountAddress)
	if !ok {
		return iotago.EmptyAccountID, ierrors.New("failed to convert account id to address")
	}

	// post issuance checks, was account really created?
	err = a.checkAccountStatus(ctx, blkID, lo.PanicOnErr(signedTx.Transaction.ID()), implicitOutputID, accountAddress, accID)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to confirm account creation")
	}
	a.registerAccount(params.Alias, implicitOutputID, addrIndex, privKey)

	return accID, nil
}

func (a *AccountWallet) createImplicitOutputTransaction(inputs []*models.Output, implicitAddress iotago.DirectUnlockableAddress, outputBalance iotago.BaseToken, rmc iotago.Mana, latestCommitmentID iotago.CommitmentID) (*iotago.SignedTransaction, error) {
	if implicitAddress.Type() != iotago.AddressImplicitAccountCreation {
		return nil, ierrors.New("invalid address type")
	}

	// minimum storage deposit for the account
	minAmount := utils.MinIssuerAccountAmount(a.client.CommittedAPI().ProtocolParameters())
	if outputBalance < minAmount {
		return nil, ierrors.Errorf("not enough funds: %d to create an account, minimum storage deposit for the account is: %d", outputBalance, minAmount)
	}

	currentTime := time.Now()
	currentSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(currentTime)

	apiForSlot := a.client.APIForSlot(currentSlot)
	txBuilder := builder.NewTransactionBuilder(apiForSlot)

	txBuilder.SetCreationSlot(currentSlot)

	for _, output := range inputs {
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: output.Address,
			InputID:      output.OutputID,
			Input:        output.OutputStruct,
		})
	}
	txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: latestCommitmentID})

	implicitOutput := builder.NewBasicOutputBuilder(implicitAddress, outputBalance).MustBuild()
	txBuilder.AddOutput(implicitOutput)

	// allot required mana to the implicit account
	a.logMissingMana(txBuilder, rmc, a.faucet.account)
	txBuilder.AllotAllMana(txBuilder.CreationSlot(), a.faucet.account.ID())

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

// createAccountImplicitly creates an implicit account by sending a faucet request for ImplicitAddressCreation funds.
// TODO this one, comparing to the non-faucet way, is not working
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

	// we await for the tx slot to be committed, but in fact the block issuance slot would be better, but we don't have it
	err = a.checkAccountStatus(ctx, iotago.EmptyBlockID, implicitAccountOutput.OutputID.TransactionID(), implicitOutputID, accountAddress, accID)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to confirm account creation")
	}

	a.registerAccount(params.Alias, implicitAccountOutput.OutputID, implicitAccountOutput.AddressIndex, implicitAccountOutput.PrivateKey)

	//if !params.Transition {
	//	//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	//	accountAddress := iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID).ToAddress().(*iotago.AccountAddress)
	//
	//	a.registerAccount(params.Alias, implicitAccountOutput.OutputID, implicitAccountOutput.AddressIndex, privateKey)
	//	log.Infof("Implicit account created, outputID: %s, implicit accountID: %s", implicitAccountOutput.OutputID.ToHex(), accountAddress.Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))
	//
	//	return iotago.EmptyAccountID, nil
	//}
	//
	//log.Debugf("Transitioning implicit account with implicitAccountID %s for alias %s to regular account", iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID).ToHex(), params.Alias)
	//
	//pubKey, isEd25519 := privateKey.Public().(ed25519.PublicKey)
	//if !isEd25519 {
	//	return iotago.EmptyAccountID, ierrors.New("Failed to create account: only Ed25519 keys are supported")
	//}
	//
	//implicitAccAddr := iotago.ImplicitAccountCreationAddressFromPubKey(pubKey)
	//implicitBlockIssuerKey := iotago.Ed25519PublicKeyHashBlockIssuerKeyFromImplicitAccountCreationAddress(implicitAccAddr)
	//blockIssuerKeys := iotago.NewBlockIssuerKeys(implicitBlockIssuerKey)
	//
	//return a.transitionImplicitAccount(ctx, implicitAccountOutput, blockIssuerKeys, privateKey, params)

	return iotago.EmptyAccountID, nil
}

func (a *AccountWallet) transitionImplicitAccount(ctx context.Context, implicitAccountOutput *models.Output, blockIssuerKeys iotago.BlockIssuerKeys, privateKey ed25519.PrivateKey, params *CreateAccountParams) (iotago.AccountID, error) {
	// in case mana in one faucet request is not enough to issue immediately
	additionalBasicInputs, err := a.requestEnoughManaForAccountCreation(ctx, iotago.Mana(implicitAccountOutput.OutputStruct.BaseTokenAmount()))
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request enough funds for account creation")
	}

	tokenBalance := implicitAccountOutput.OutputStruct.BaseTokenAmount() + utils.SumOutputsBalance(additionalBasicInputs)

	// transition from implicit to regular account
	accAddr, accPrivateKey, accAddrIndex := a.getAddress(iotago.AddressEd25519)
	log.Infof("Address generated for account: %s", accAddr)
	accountOutput := builder.NewAccountOutputBuilder(accAddr, tokenBalance).
		AccountID(iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID)).
		BlockIssuer(blockIssuerKeys, iotago.MaxSlotIndex).MustBuild()

	log.Infof("Created account %s with %d tokens\n", accountOutput.AccountID.ToHex(), accountOutput.Amount)

	// transaction preparation
	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(ctx, a.client, a.faucet.account)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}
	inputs := append([]*models.Output{implicitAccountOutput}, additionalBasicInputs...)
	signedTx, err := a.createAccountCreationTransaction(inputs, accountOutput, congestionResp, issuerResp)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to create account creation transaction")
	}

	log.Debugf("Transition transaction created:\n%s\n", utils.SprintTransaction(signedTx))

	implicitAccountHandler := wallet.NewEd25519Account(iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID), privateKey)
	blkID, err := a.PostWithBlock(ctx, a.client, signedTx, implicitAccountHandler, congestionResp, issuerResp, version)
	if err != nil {
		log.Errorf("Failed to post account with block: %s", err)

		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to post transaction")
	}
	log.Debugf("Block sent with ID: %s, and no error", blkID.ToHex())
	accID := iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID)
	//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	accountAddress := accID.ToAddress().(*iotago.AccountAddress)
	err = a.checkAccountStatus(ctx, blkID, lo.PanicOnErr(signedTx.Transaction.ID()), implicitAccountOutput.OutputID, accountAddress, accID)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}

	a.registerAccount(params.Alias, implicitAccountOutput.OutputID, accAddrIndex, accPrivateKey)

	return accID, nil
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
	accID := iotago.AccountIDFromOutputID(creationOutput.OutputID)
	fmt.Printf("Prepared account %s, from input: %s, account: %s\n", accID.ToHex(), creationOutput.OutputID.String(), utils.SprintAccount(accountOutput))

	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(ctx, a.client, a.faucet.account)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}

	signedTx, err := a.createAccountCreationTransaction([]*models.Output{creationOutput}, accountOutput, congestionResp, issuerResp)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to create account transaction")
	}
	log.Debugf("Transaction for account creation signed: %s\n", utils.SprintTransaction(signedTx))
	blkID, err := a.PostWithBlock(ctx, a.client, signedTx, a.faucet.account, congestionResp, issuerResp, version)
	if err != nil {
		log.Errorf("Failed to post account with block: %s", err)

		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to post transaction")
	}
	log.Debugf("Account creation transaction posted with block: %s, with no error.\n", blkID.ToHex())
	//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	accountID := iotago.AccountIDFromOutputID(creationOutput.OutputID)
	accountAddress := accountID.ToAddress().(*iotago.AccountAddress)
	err = a.checkAccountStatus(ctx, blkID, lo.PanicOnErr(signedTx.Transaction.ID()), creationOutput.OutputID, accountAddress, accountID)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}

	a.registerAccount(params.Alias, creationOutput.OutputID, accAddrIndex, accPrivateKey)

	return accID, nil
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

	// allot required mana to the implicit account
	a.logMissingMana(txBuilder, congestionResp.ReferenceManaCost, a.faucet.account)
	txBuilder.AllotAllMana(txBuilder.CreationSlot(), a.faucet.account.ID())

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
