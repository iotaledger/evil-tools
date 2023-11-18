package accountwallet

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"fmt"
	"sync"
	"time"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

// CreateAccount creates an implicit account and immediately transition it to a regular account.
func (a *AccountWallet) CreateAccount(ctx context.Context, params *CreateAccountParams) (iotago.AccountID, error) {
	if params.Implicit {
		return a.createAccountImplicitly(ctx, params)
	}

	return a.createAccountWithFaucet(ctx, params)
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

	implicitAccountID := iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID)

	if !params.Transition {
		a.registerAccount(params.Alias, implicitAccountOutput.OutputID, implicitAccountOutput.AddressIndex, privateKey)
		log.Infof("Implicit account created, outputID: %s, implicit accountID: %s", implicitAccountOutput.OutputID.ToHex(), implicitAccountID.ToHex())

		return implicitAccountID, nil
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
	additionalBasicInputs, _ := a.requestEnoughFundsForAccountCreation(ctx, iotago.Mana(implicitAccountOutput.Balance))
	tokenBalance := implicitAccountOutput.Balance + utils.SumOutputsBalance(additionalBasicInputs)

	// transition from implicit to regular account
	accAddr, accPrivateKey, accAddrIndex := a.getAddress(iotago.AddressEd25519)
	log.Infof("Address generated for account: %s", accAddr)
	accountOutput := builder.NewAccountOutputBuilder(accAddr, tokenBalance).
		Mana(implicitAccountOutput.OutputStruct.StoredMana()).
		//AccountID(iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID)).
		BlockIssuer(blockIssuerKeys, iotago.MaxSlotIndex).MustBuild()

	log.Infof("Created account %s with %d tokens\n", accountOutput.AccountID.ToHex(), accountOutput.Amount)

	// transaction preparation
	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(ctx, a.client.Client(), a.faucet.account.ID())
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}
	//inputs := append([]*models.Output{implicitAccountOutput}, additionalBasicInputs...)
	inputs := []*models.Output{implicitAccountOutput}
	signedTx, err := a.createAccountCreationTransaction(inputs, accountOutput, congestionResp, issuerResp)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to create account creation transaction")
	}
	log.Info(utils.SprintTransaction(signedTx))

	implicitAccountHandler := mock.NewEd25519Account(iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID), privateKey)
	blkID, err := a.PostWithBlock(ctx, a.client, signedTx, implicitAccountHandler, congestionResp, issuerResp, version)
	if err != nil {
		log.Errorf("Failed to post account with block: %s", err)

		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to post transaction")
	}

	accountID := iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID)
	err = a.checkAccountStatus(ctx, blkID, accountID)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}

	a.registerAccount(params.Alias, implicitAccountOutput.OutputID, accAddrIndex, accPrivateKey)

	return accountID, nil
}

func (a *AccountWallet) requestEnoughFundsForAccountCreation(ctx context.Context, currentMana iotago.Mana) ([]*models.Output, error) {
	requiredMana, err := a.estimateMinimumRequiredMana(ctx, 1, 0, false, true)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to estimate number of faucet requests to cover minimum required mana")
	}
	log.Debugf("Mana required for account creation: %d, requesting additional mana from the faucet", requiredMana)
	if currentMana >= requiredMana {
		return []*models.Output{}, nil
	}
	additionalBasicInputs, err := a.RequestManaFromTheFaucet(ctx, requiredMana)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to request mana from the faucet")
	}
	log.Debugf("successfully requested %d new outputs from the faucet", len(additionalBasicInputs))

	return additionalBasicInputs, nil
}

func (a *AccountWallet) RequestManaFromTheFaucet(ctx context.Context, minManaAmount iotago.Mana) ([]*models.Output, error) {
	if minManaAmount/a.faucet.RequestManaAmount > MaxFaucetManaRequests {
		return nil, ierrors.Errorf("required mana is too large, needs more than %d faucet requests", MaxFaucetManaRequests)
	}

	outputs := make([]*models.Output, 0)
	wg := sync.WaitGroup{}
	// if there is not enough mana to pay for account creation, request mana from the faucet
	for requested := iotago.Mana(0); requested < minManaAmount; requested += a.faucet.RequestManaAmount {
		wg.Add(1)
		go func(requested iotago.Mana) {
			defer wg.Done()

			log.Debugf("Requesting %d mana from the faucet, already requested %d", a.faucet.RequestManaAmount, requested)
			faucetOutput, _, err := a.getFunds(ctx, iotago.AddressEd25519)
			if err != nil {
				log.Errorf("failed to request funds from the faucet: %v", err)
			}
			outputs = append(outputs, faucetOutput)
		}(requested)
	}
	wg.Wait()

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
	accountOutput := builder.NewAccountOutputBuilder(accAddr, creationOutput.Balance).
		//Mana(manaBalance). this one will be updated after allotment
		//AccountID no accountID should be specified during the account creation
		BlockIssuer(blockIssuerKeys, iotago.MaxSlotIndex).MustBuild()

	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(ctx, a.client.Client(), a.faucet.account.ID())
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
	accountID := iotago.AccountIDFromOutputID(creationOutput.OutputID)

	err = a.checkAccountStatus(ctx, blkID, accountID)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failure in account creation")
	}

	a.registerAccount(params.Alias, creationOutput.OutputID, accAddrIndex, accPrivateKey)

	return accountID, nil
}

func (a *AccountWallet) checkAccountStatus(ctx context.Context, blkID iotago.BlockID, accountID iotago.AccountID) error {
	if err := utils.AwaitBlockAndPayloadAcceptance(ctx, a.client, blkID); err != nil {
		return ierrors.Wrapf(err, "failed to await block issuance for block %s", blkID.ToHex())
	}

	log.Infof("Created account %s, blk ID %s, awaiting the commitment.", accountID.ToHex(), blkID.ToHex())
	// wait for the account to be committed
	err := utils.AwaitCommitment(ctx, a.client, blkID.Slot())
	if err != nil {
		log.Errorf("Failed to await commitment for slot %d: %s", blkID.Slot(), err)

		return err
	}
	log.Infof("Slot %d is committed", blkID.Slot())
	// make sure it exists, and get the details from the indexer
	outputID, account, slot, err := a.client.GetAccountFromIndexer(ctx, accountID)
	if err != nil {
		log.Debugf("Failed to get account from indexer, even after slot %d is already committed", blkID.Slot())

	log.Infof("Account created, ID: %s, slot: %d", accountID.ToHex(), blkID.Slot())

	return nil
}

func (a *AccountWallet) createAccountCreationTransaction(inputs []*models.Output, accountOutput *iotago.AccountOutput, congestionResp *apimodels.CongestionResponse, issuerResp *apimodels.IssuanceBlockHeaderResponse) (*iotago.SignedTransaction, error) {
	// transaction preparation

	//inputs := append([]*models.Output{implicitAccountOutput}, additionalBasicInputs...)
	commitmentID, _ := issuerResp.Commitment.ID()
	txBuilder := a.createTransactionBuilder(inputs, accountOutput, commitmentID)

	// allot required mana to the implicit account
	a.logMissingMana(txBuilder, congestionResp.ReferenceManaCost, a.faucet.account.ID())
	txBuilder.AllotRequiredManaAndStoreRemainingManaInOutput(txBuilder.CreationSlot(), congestionResp.ReferenceManaCost, a.faucet.account.ID(), 0)

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

func (a *AccountWallet) createTransactionBuilder(inputs []*models.Output, accountOutput *iotago.AccountOutput, commitmentID iotago.CommitmentID) *builder.TransactionBuilder {
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

	// needed for BIF
	txBuilder.AddContextInput(&iotago.CommitmentInput{CommitmentID: commitmentID})

	return txBuilder
}

func (a *AccountWallet) estimateMinimumRequiredMana(ctx context.Context, basicInputCount, basicOutputCount int, accountInput bool, accountOutput bool) (iotago.Mana, error) {
	fmt.Print(a.faucet.account.ID())
	congestionResp, err := a.client.GetCongestion(ctx, a.faucet.account.ID())
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

func (a *AccountWallet) logMissingMana(finishedTxBuilder *builder.TransactionBuilder, rmc iotago.Mana, issuerAccountID iotago.AccountID) {
	availableMana, err := finishedTxBuilder.CalculateAvailableMana(finishedTxBuilder.CreationSlot())
	if err != nil {
		log.Error("could not calculate available mana")

		return
	}
	log.Debug(utils.SprintAvailableManaResult(availableMana))
	minRequiredAllottedMana, err := finishedTxBuilder.MinRequiredAllotedMana(a.client.APIForSlot(finishedTxBuilder.CreationSlot()).ProtocolParameters().WorkScoreParameters(), rmc, issuerAccountID)
	if err != nil {
		log.Error("could not calculate min required allotted mana")

		return
	}
	log.Debugf("Min required allotted mana: %d", minRequiredAllottedMana)
}

func (a *AccountWallet) getAddress(addressType iotago.AddressType) (iotago.DirectUnlockableAddress, ed25519.PrivateKey, uint64) {
	newIndex := a.latestUsedIndex.Inc()
	hdWallet := mock.NewKeyManager(a.seed[:], newIndex)
	privKey, _ := hdWallet.KeyPair()
	receiverAddr := hdWallet.Address(addressType)

	return receiverAddr, privKey, newIndex
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
			addrKeys = append(addrKeys, iotago.NewAddressKeysForEd25519Address(ed25519Addr, out.PrivKey))
		case iotago.AddressImplicitAccountCreation:
			implicitAccountCreationAddr, ok := out.Address.(*iotago.ImplicitAccountCreationAddress)
			if !ok {
				return nil, ierrors.New("failed type ImplicitAccountCreationAddress assertion, invalid address type")
			}
			addrKeys = append(addrKeys, iotago.NewAddressKeysForImplicitAccountCreationAddress(implicitAccountCreationAddr, out.PrivKey))
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
		fmt.Printf("%-33s ", accData.Account.ID().ToHex())
		fmt.Printf("\n")
	}

	return nil
}

func (a *AccountWallet) AllotToAccount(_ *AllotAccountParams) error {
	return nil
}
