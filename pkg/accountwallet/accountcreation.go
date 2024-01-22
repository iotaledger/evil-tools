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
	iotagowallet "github.com/iotaledger/iota.go/v4/wallet"
)

// createImplicitAccount creates an implicit account by sending a faucet request for ImplicitAddressCreation funds.
func (a *AccountWallets) createImplicitAccount(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	a.LogDebug("Creating an implicit account")
	// An implicit account has an implicitly defined Block Issuer Key, corresponding to the address itself.
	// Thus, implicit accounts can issue blocks by signing them with the private key corresponding to the public key
	// from which the Implicit Account Creation Address was derived.
	wallet := a.GetOrCreateWallet(params.Alias)
	implicitAccountOutput, err := wallet.getFaucetFundsOutput(ctx, iotago.AddressImplicitAccountCreation)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "Failed to create account")
	}

	implicitOutputID := implicitAccountOutput.OutputID
	accID := iotago.AccountIDFromOutputID(implicitOutputID)
	accountAddress, ok := accID.ToAddress().(*iotago.AccountAddress)
	if !ok {
		return iotago.EmptyAccountID, ierrors.New("failed to convert account id to address")
	}
	a.LogDebugf("Created implicit account output, outputID: %s", implicitOutputID.ToHex())
	a.LogInfof("Posted transaction: implicit account output from faucet\nBech addr: %s", accountAddress.Bech32(a.Client.CommittedAPI().ProtocolParameters().Bech32HRP()))

	err = wallet.checkOutputStatus(ctx, iotago.EmptyBlockID, implicitAccountOutput.OutputID.TransactionID(), implicitOutputID, accountAddress)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}
	a.registerAccount(params.Alias, accID, implicitOutputID, implicitAccountOutput.AddressIndex, implicitAccountOutput.PrivateKey)

	if !params.Transition {
		a.LogInfof("Implicit account created, outputID: %s, implicit accountID: %s", implicitAccountOutput.OutputID.ToHex(), accountAddress.Bech32(a.Client.CommittedAPI().ProtocolParameters().Bech32HRP()))

		return accID, nil
	}

	return a.transitionImplicitAccount(ctx, implicitAccountOutput, params)
}

//nolint:forcetypeassert
func (a *AccountWallets) transitionImplicitAccount(ctx context.Context, implicitAccountOutput *models.OutputData, params *CreateAccountParams) (iotago.AccountID, error) {
	a.LogDebugf("Transitioning implicit account with implicitAccountID %s for alias %s to regular account", iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID).ToHex(), params.Alias)

	tokenBalance := implicitAccountOutput.OutputStruct.BaseTokenAmount()
	accountID := iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID)
	accountAddress := accountID.ToAddress().(*iotago.AccountAddress)

	// build account output with new Ed25519 address
	wallet := a.GetOrCreateWallet(params.Alias)
	accEd25519Addr, accPrivateKey, accAddrIndex := wallet.getAddress(iotago.AddressEd25519)
	accBlockIssuerKey := iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(accPrivateKey.Public().(ed25519.PublicKey))
	accountOutput := builder.NewAccountOutputBuilder(accEd25519Addr, tokenBalance).
		AccountID(accountID).
		BlockIssuer(iotago.NewBlockIssuerKeys(accBlockIssuerKey), iotago.MaxSlotIndex).MustBuild()

	// transaction preparation, issue block with implicit account
	implicitAccountIssuer := iotagowallet.NewEd25519Account(accountID, implicitAccountOutput.PrivateKey)
	congestionResp, issuerResp, version, err := wallet.RequestBlockIssuanceData(ctx, a.Client, implicitAccountIssuer)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request block built data for the implicit account")
	}

	signedTx, err := wallet.createAccountCreationTransaction([]*models.OutputData{implicitAccountOutput}, accountOutput, congestionResp, issuerResp)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to create account creation transaction")
	}
	a.LogDebugf("Implicit account transition transaction created:\n%s\n", utils.SprintTransaction(a.Client.LatestAPI(), signedTx))

	// post block with implicit account
	blkID, err := wallet.PostWithBlock(ctx, a.Client, signedTx, implicitAccountIssuer, congestionResp, issuerResp, version)
	if err != nil {
		a.LogErrorf("Failed to post account with block: %s", err)

		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to post transaction")
	}
	a.LogDebugf("Block sent with ID: %s, and no error", blkID.ToHex())

	// get OutputID of account output
	accOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
	a.LogInfof("Posted transaction: transition implicit account to full account\nBech addr: %s", accountAddress.Bech32(a.Client.CommittedAPI().ProtocolParameters().Bech32HRP()))
	err = wallet.checkOutputStatus(ctx, blkID, lo.PanicOnErr(signedTx.Transaction.ID()), accOutputID, accountAddress, true)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}

	// update account output, address and private key
	a.registerAccount(params.Alias, accountID, accOutputID, accAddrIndex, accPrivateKey)

	return accountID, nil
}

func (a *AccountWallets) createAccountWithFaucet(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	wallet := a.GetOrCreateWallet(params.Alias)
	a.LogDebug("Creating an account with faucet funds and mana")
	// request enough funds to cover the mana required for account creation
	creationOutput, err := wallet.getFaucetFundsOutput(ctx, iotago.AddressEd25519)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request enough funds for account creation")
	}
	accAddr, accPrivateKey, accAddrIndex := wallet.getAddress(iotago.AddressEd25519)

	blockIssuerKeys, err := wallet.getAccountPublicKeys(accPrivateKey.Public())
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to get account address and keys")
	}
	accountOutput := builder.NewAccountOutputBuilder(accAddr, creationOutput.OutputStruct.BaseTokenAmount()).
		// mana  will be updated after allotment
		// no accountID should be specified during the account creation
		BlockIssuer(blockIssuerKeys, iotago.MaxSlotIndex).MustBuild()

	congestionResp, issuerResp, version, err := wallet.RequestBlockIssuanceData(ctx, a.Client, a.GenesisAccount)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}

	signedTx, err := wallet.createAccountCreationTransaction([]*models.OutputData{creationOutput}, accountOutput, congestionResp, issuerResp)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to create account transaction")
	}
	a.LogDebugf("Transaction for account creation signed: %s\n", utils.SprintTransaction(a.Client.LatestAPI(), signedTx))

	blkID, err := wallet.PostWithBlock(ctx, a.Client, signedTx, a.GenesisAccount, congestionResp, issuerResp, version)
	if err != nil {
		a.LogErrorf("Failed to post account with block: %s", err)

		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to post transaction")
	}
	a.LogDebugf("Account creation transaction posted with block: %s, in slot: %d with no error.\n", blkID.ToHex(), blkID.Slot())

	accountOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
	accountID := iotago.AccountIDFromOutputID(accountOutputID)
	//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	accountAddress := accountID.ToAddress().(*iotago.AccountAddress)
	err = wallet.checkOutputStatus(ctx, blkID, lo.PanicOnErr(signedTx.Transaction.ID()), accountOutputID, accountAddress, true)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}

	a.registerAccount(params.Alias, accountID, accountOutputID, accAddrIndex, accPrivateKey)

	return accountID, nil
}

func (a *AccountWallet) createAccountCreationTransaction(inputs []*models.OutputData, accountOutput *iotago.AccountOutput, congestionResp *api.CongestionResponse, issuerResp *api.IssuanceBlockHeaderResponse) (*iotago.SignedTransaction, error) {
	currentTime := time.Now()
	currentSlot := a.Client.LatestAPI().TimeProvider().SlotFromTime(currentTime)
	apiForSlot := a.Client.APIForSlot(currentSlot)

	// empty accountID means that the account is created from the faucet account
	accountID := accountOutput.AccountID
	if accountID == iotago.EmptyAccountID {
		accountID = a.GenesisAccount.ID()
	}

	// transaction signer
	addrSigner, err := a.GetAddrSignerForIndexes(inputs...)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get address signer")
	}

	txBuilder := builder.NewTransactionBuilder(apiForSlot)
	for _, output := range inputs {
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: output.Address,
			InputID:      output.OutputID,
			Input:        output.OutputStruct,
		})
	}

	txBuilder.
		AddOutput(accountOutput).
		SetCreationSlot(currentSlot).
		AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: lo.Return1(issuerResp.LatestCommitment.ID())}).AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: accountID}).
		WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything())).AllotAllMana(txBuilder.CreationSlot(), accountID)

	// allot required mana to the implicit account
	a.logMissingMana(txBuilder, congestionResp.ReferenceManaCost, accountID)

	signedTx, err := txBuilder.Build(addrSigner)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to build tx")
	}

	return signedTx, nil
}
