package accountwallet

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"fmt"
	"math"
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

// CreateAccount creates an implicit account and immediately transition it to a regular account.
func (a *AccountWallet) CreateAccount(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	if params.Implicit {
		err := a.createImplicitAccount(ctx)
		if err != nil {
			return iotago.AccountID{}, ierrors.Wrap(err, "failed to create implicit account")
		}
		return a.createAccountImplicitly(ctx, params)
	}

	return a.createAccountWithFaucet(ctx, params)
}

func (a *AccountWallet) createImplicitAccount(ctx context.Context) error {
	log.Debug("Creating an implicit account")
	// get enough funds
	minimumTokenAmount := utils.MinIssuerAccountAmount(a.client.CommittedAPI().ProtocolParameters())

	inputs, err := a.RequestManaAndFundsFromTheFaucet(ctx, 0, minimumTokenAmount)
	if err != nil {
		return ierrors.Wrap(err, "failed to request enough funds for account creation")
	}

	balance := utils.SumOutputsBalance(inputs)
	implicitAddr, _, _ := a.getAddress(iotago.AddressImplicitAccountCreation)

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
	implicitOutput := builder.NewBasicOutputBuilder(implicitAddr, balance).MustBuild()
	//implicitModelOutput := &models.Output{
	//	Address:   implicitAddr,
	//	AddressIndex: accAddrIndex,
	//	PrivateKey: accPrivateKey,
	//	OutputStruct: implicitOutput,
	//}

	txBuilder.AddOutput(implicitOutput)
	txBuilder.SetCreationSlot(currentSlot)

	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(ctx, a.client, a.faucet.account)

	commitmentID, _ := issuerResp.Commitment.ID()
	txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: commitmentID})

	// allot required mana to the implicit account
	a.logMissingMana(txBuilder, congestionResp.ReferenceManaCost, a.faucet.account)
	txBuilder.AllotAllMana(txBuilder.CreationSlot(), a.faucet.account.ID())

	// sign the transaction
	addrSigner, err := a.getAddrSignerForIndexes(inputs...)
	if err != nil {
		return ierrors.Wrap(err, "failed to get address signer")
	}

	signedTx, err := txBuilder.Build(addrSigner)
	if err != nil {
		return ierrors.Wrap(err, "failed to build tx")
	}

	txID, err := signedTx.Transaction.ID()
	if err != nil {
		return ierrors.Wrap(err, "failed to get tx id")
	}

	blkID, err := a.PostWithBlock(ctx, a.client, signedTx, a.faucet.account, congestionResp, issuerResp, version)
	if err != nil {
		log.Errorf("Failed to post account with block: %s", err)

		return ierrors.Wrap(err, "failed to post transaction")
	}
	implicitOutputID := iotago.OutputIDFromTransactionIDAndIndex(txID, 0)

	resp, err := a.client.GetBlockStateFromTransaction(ctx, txID)
	if err != nil {
		return ierrors.Wrap(err, "failed to get block state from faucet tx")
	}
	log.Debugf("RequestFaucetFunds faucet tx state: %s, block state: %s, tx failure: %d, block failure: %d", resp.TransactionState, resp.BlockState, resp.TransactionFailureReason, resp.BlockFailureReason)

	err = utils.AwaitCommitment(ctx, a.client, blkID.Slot())
	if err != nil {
		return ierrors.Wrap(err, "failed to await commitment")
	}
	log.Debugf("slot %s is  %d committed, implicitOutputID: %s, accID: %s, blkID: %s, txID: %s", blkID.Slot(), implicitOutputID.String(), iotago.AccountIDFromOutputID(implicitOutputID).String(), blkID.String(), txID.String())

	return nil
}

func (a *AccountWallet) createAccountImplicitly(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	log.Debug("Creating an implicit account")
	// An implicit account has an implicitly defined Block Issuer Key, corresponding to the address itself.
	// Thus, implicit accounts can issue blocks by signing them with the private key corresponding to the public key
	// from which the Implicit Account Creation Address was derived.
	implicitAccountOutput, privateKey, err := a.getFunds(ctx, iotago.AddressImplicitAccountCreation)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "Failed to create account")
	}

	log.Debugf("RequestFaucetFunds faucet outputID: %s, txID: %s", implicitAccountOutput.OutputID.String(), implicitAccountOutput.OutputID.TransactionID().String())

	minAmount := utils.MinIssuerAccountAmount(a.client.CommittedAPI().ProtocolParameters())
	log.Debugf("Minimum token amount for implicit account creation: %d, our output balance %d", minAmount, implicitAccountOutput.OutputStruct.BaseTokenAmount())

	resp, err := a.client.GetBlockStateFromTransaction(ctx, implicitAccountOutput.OutputID.TransactionID())
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to get block state from faucet tx")
	}
	log.Debugf("RequestFaucetFunds faucet tx state: %s, block state: %s, tx failure: %d, block failure: %d", resp.TransactionState, resp.BlockState, resp.TransactionFailureReason, resp.BlockFailureReason)

	err = utils.AwaitCommitment(ctx, a.client, implicitAccountOutput.OutputID.TransactionID().Slot()+2)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to await commitment")
	}
	log.Debugf("slot of implicitOutput: %s is  %d committed", implicitAccountOutput.OutputID.String(), implicitAccountOutput.OutputID.TransactionID().Slot())

	if !params.Transition {
		//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
		accountAddress := iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID).ToAddress().(*iotago.AccountAddress)

		a.registerAccount(params.Alias, implicitAccountOutput.OutputID, implicitAccountOutput.AddressIndex, privateKey)
		log.Infof("Implicit account created, outputID: %s, implicit accountID: %s", implicitAccountOutput.OutputID.ToHex(), accountAddress.Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))

		return iotago.EmptyAccountID, nil
	}

	log.Debugf("Transitioning implicit account with implicitAccountID %s for alias %s to regular account", params.Alias, iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID).ToHex())

	pubKey, isEd25519 := privateKey.Public().(ed25519.PublicKey)
	if !isEd25519 {
		return iotago.EmptyAccountID, ierrors.New("Failed to create account: only Ed25519 keys are supported")
	}

	implicitAccAddr := iotago.ImplicitAccountCreationAddressFromPubKey(pubKey)
	implicitBlockIssuerKey := iotago.Ed25519PublicKeyHashBlockIssuerKeyFromImplicitAccountCreationAddress(implicitAccAddr)
	blockIssuerKeys := iotago.NewBlockIssuerKeys(implicitBlockIssuerKey)

	return a.transitionImplicitAccount(ctx, implicitAccountOutput, blockIssuerKeys, privateKey, params)
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

	log.Info(utils.SprintTransaction(signedTx))

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
	err = a.checkAccountStatus(ctx, blkID, accountAddress)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}

	a.registerAccount(params.Alias, implicitAccountOutput.OutputID, accAddrIndex, accPrivateKey)

	return accID, nil
}

func (a *AccountWallet) requestEnoughManaForAccountCreation(ctx context.Context, currentMana iotago.Mana) ([]*models.Output, error) {
	requiredMana, err := a.estimateMinimumRequiredMana(ctx, 1, 0, false, true)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to estimate number of faucet requests to cover minimum required mana")
	}
	log.Debugf("Mana required for account creation: %d, requesting additional mana from the faucet", requiredMana)
	if currentMana >= requiredMana {
		return []*models.Output{}, nil
	}
	additionalBasicInputs, err := a.RequestManaAndFundsFromTheFaucet(ctx, requiredMana, 0)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to request mana from the faucet")
	}
	log.Debugf("successfully requested %d new outputs from the faucet", len(additionalBasicInputs))

	return additionalBasicInputs, nil
}

func (a *AccountWallet) RequestManaAndFundsFromTheFaucet(ctx context.Context, minManaAmount iotago.Mana, minTokensAmount iotago.BaseToken) ([]*models.Output, error) {
	if minManaAmount/a.faucet.RequestManaAmount > MaxFaucetRequestsForOneOperation {
		return nil, ierrors.Errorf("required mana is too large, needs more than %d faucet requests", MaxFaucetRequestsForOneOperation)
	}

	if minTokensAmount/a.faucet.RequestTokenAmount > MaxFaucetRequestsForOneOperation {
		return nil, ierrors.Errorf("required token amount is too large, needs more than %d faucet requests", MaxFaucetRequestsForOneOperation)
	}
	numOfRequests := int(math.Max(float64(minManaAmount/a.faucet.RequestManaAmount), float64(minTokensAmount/a.faucet.RequestTokenAmount)))

	outputsChan := make(chan *models.Output)
	doneChan := make(chan struct{})
	// if there is not enough mana to pay for account creation, request mana from the faucet
	for i := 0; i < numOfRequests; i++ {
		go func(ctx context.Context, outputs chan *models.Output, doneChan chan struct{}) {
			defer func() {
				doneChan <- struct{}{}
			}()

			faucetOutput, _, err := a.getFunds(ctx, iotago.AddressEd25519)
			if err != nil {
				log.Errorf("failed to request funds from the faucet: %v", err)

				return
			}

			outputsChan <- faucetOutput
		}(ctx, outputsChan, doneChan)
	}

	outputs := make([]*models.Output, 0)
	for requestsProcessed := 0; requestsProcessed < numOfRequests; {
		select {
		case faucetOutput := <-outputsChan:
			log.Debugf("Faucet requests, received faucet output: %s", faucetOutput.OutputID.ToHex())
			outputs = append(outputs, faucetOutput)
		case <-doneChan:
			requestsProcessed++
		}
	}

	return outputs, nil
}

func (a *AccountWallet) createAccountWithFaucet(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	log.Debug("Creating an account with faucet funds and mana")
	// request enough funds to cover the mana required for account creation
	creationOutput, _, err := a.getFunds(ctx, iotago.AddressEd25519)
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

	blkID, err := a.PostWithBlock(ctx, a.client, signedTx, a.faucet.account, congestionResp, issuerResp, version)
	if err != nil {
		log.Errorf("Failed to post account with block: %s", err)

		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to post transaction")
	}

	//nolint:forcetypeassert // we know that the address is of type *iotago.AccountAddress
	accountAddress := iotago.AccountIDFromOutputID(creationOutput.OutputID).ToAddress().(*iotago.AccountAddress)
	err = a.checkAccountStatus(ctx, blkID, accountAddress)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}

	a.registerAccount(params.Alias, creationOutput.OutputID, accAddrIndex, accPrivateKey)

	return accID, nil
}

func (a *AccountWallet) checkAccountStatus(ctx context.Context, blkID iotago.BlockID, accountAddress *iotago.AccountAddress) error {
	if err := utils.AwaitBlockAndPayloadAcceptance(ctx, a.client, blkID); err != nil {
		return ierrors.Wrapf(err, "failed to await block issuance for block %s", blkID.ToHex())
	}

	log.Infof("Created account %s, blk ID %s, awaiting the commitment.", accountAddress.Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()), blkID.ToHex())
	// wait for the account to be committed
	err := utils.AwaitCommitment(ctx, a.client, blkID.Slot())
	if err != nil {
		log.Errorf("Failed to await commitment for slot %d: %s", blkID.Slot(), err)

		return err
	}
	log.Infof("Slot %d is committed", blkID.Slot())
	// TODO the indexer is not returning the account details
	// make sure it exists, and get the details from the indexer
	//outputID, account, slot, err := a.client.GetAccountFromIndexer(ctx, accountID)
	//if err != nil {
	//	log.Debugf("Failed to get account from indexer, even after slot %d is already committed", blkID.Slot())
	//
	//	return err
	//}

	log.Infof("Account created, ID: %s, slot: %d", accountAddress.Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()), blkID.Slot())

	return nil
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

	commitmentID, _ := issuerResp.Commitment.ID()
	txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: commitmentID})

	// allot required mana to the implicit account
	a.logMissingMana(txBuilder, congestionResp.ReferenceManaCost, a.faucet.account)
	txBuilder.AllotAllMana(txBuilder.CreationSlot(), a.faucet.account.ID())

	// sign the transaction
	addrSigner, err := a.getAddrSignerForIndexes(inputs...)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get address signer")
	}

	signedTx, err := txBuilder.Build(addrSigner)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to build tx")
	}

	return signedTx, nil
}

func (a *AccountWallet) estimateMinimumRequiredMana(ctx context.Context, basicInputCount, basicOutputCount int, accountInput bool, accountOutput bool) (iotago.Mana, error) {
	congestionResp, err := a.client.GetCongestion(ctx, a.faucet.account.Address())
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to get congestion data for faucet accountID")
	}

	txBuilder := utils.PrepareDummyTransactionBuilder(a.client.APIForSlot(congestionResp.Slot), basicInputCount, basicOutputCount, accountInput, accountOutput)
	minRequiredAllottedMana, err := txBuilder.MinRequiredAllotedMana(a.client.APIForSlot(congestionResp.Slot).ProtocolParameters().WorkScoreParameters(), congestionResp.ReferenceManaCost, iotago.EmptyAccountID)
	if err != nil {
		return 0, ierrors.Wrap(err, "could not calculate min required allotted mana")
	}

	return minRequiredAllottedMana, nil
}

func (a *AccountWallet) logMissingMana(finishedTxBuilder *builder.TransactionBuilder, rmc iotago.Mana, issuer wallet.Account) {
	availableMana, err := finishedTxBuilder.CalculateAvailableMana(finishedTxBuilder.CreationSlot())
	if err != nil {
		log.Error("could not calculate available mana")

		return
	}
	log.Debug(utils.SprintAvailableManaResult(availableMana))
	minRequiredAllottedMana, err := finishedTxBuilder.MinRequiredAllotedMana(a.client.APIForSlot(finishedTxBuilder.CreationSlot()).ProtocolParameters().WorkScoreParameters(), rmc, issuer.Address().AccountID())
	if err != nil {
		log.Error("could not calculate min required allotted mana")

		return
	}
	log.Debugf("Min required allotted mana: %d", minRequiredAllottedMana)
}

func (a *AccountWallet) getAddress(addressType iotago.AddressType) (iotago.DirectUnlockableAddress, ed25519.PrivateKey, uint64) {
	newIndex := a.latestUsedIndex.Inc()
	keyManager := lo.PanicOnErr(wallet.NewKeyManager(a.seed[:], newIndex))
	privateKey, _ := keyManager.KeyPair()
	receiverAddr := keyManager.Address(addressType)

	return receiverAddr, privateKey, newIndex
}

func (a *AccountWallet) getAccountPublicKeys(pubKey crypto.PublicKey) (iotago.BlockIssuerKeys, error) {
	ed25519PubKey, isEd25519 := pubKey.(ed25519.PublicKey)
	if !isEd25519 {
		return nil, ierrors.New("Failed to create account: only Ed25519 keys are supported")
	}

	blockIssuerKeys := iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(ed25519PubKey))

	return blockIssuerKeys, nil

}

func (a *AccountWallet) getAddrSignerForIndexes(outputs ...*models.Output) (iotago.AddressSigner, error) {
	var addrKeys []iotago.AddressKeys
	for _, out := range outputs {
		switch out.Address.Type() {
		case iotago.AddressEd25519:
			ed25519Addr, ok := out.Address.(*iotago.Ed25519Address)
			if !ok {
				return nil, ierrors.New("failed Ed25519Address type assertion, invalid address type")
			}
			addrKeys = append(addrKeys, iotago.NewAddressKeysForEd25519Address(ed25519Addr, out.PrivateKey))
		case iotago.AddressImplicitAccountCreation:
			implicitAccountCreationAddr, ok := out.Address.(*iotago.ImplicitAccountCreationAddress)
			if !ok {
				return nil, ierrors.New("failed type ImplicitAccountCreationAddress assertion, invalid address type")
			}
			addrKeys = append(addrKeys, iotago.NewAddressKeysForImplicitAccountCreationAddress(implicitAccountCreationAddr, out.PrivateKey))
		}
	}

	return iotago.NewInMemoryAddressSigner(addrKeys...), nil
}

func (a *AccountWallet) DestroyAccount(ctx context.Context, params *DestroyAccountParams) error {
	return a.destroyAccount(ctx, params.AccountAlias)
}

func (a *AccountWallet) ListAccount() error {
	a.accountAliasesMutex.RLock()
	defer a.accountAliasesMutex.RUnlock()

	fmt.Printf("%-10s \t%-33s\n\n", "Alias", "AccountID")
	for _, accData := range a.accountsAliases {
		fmt.Printf("%-10s \t", accData.Alias)
		fmt.Printf("%-33s ", accData.Account.Address().AccountID().ToHex())
		fmt.Printf("\n")
	}

	return nil
}

func (a *AccountWallet) AllotToAccount(_ *AllotAccountParams) error {
	return nil
}
