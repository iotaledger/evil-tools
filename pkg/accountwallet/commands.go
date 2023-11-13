package accountwallet

import (
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
	"github.com/iotaledger/iota.go/v4/tpkg"
)

// CreateAccount creates an implicit account and immediately transition it to a regular account.
func (a *AccountWallet) CreateAccount(params *CreateAccountParams) (iotago.AccountID, error) {
	if params.Implicit {
		return a.createAccountImplicitly(params)
	}

	return a.createAccountWithFaucet(params)
}

func (a *AccountWallet) createAccountImplicitly(params *CreateAccountParams) (iotago.AccountID, error) {
	log.Debug("Creating an implicit account")
	// An implicit account has an implicitly defined Block Issuer Key, corresponding to the address itself.
	// Thus, implicit accounts can issue blocks by signing them with the private key corresponding to the public key
	// from which the Implicit Account Creation Address was derived.
	implicitAccountOutput, privateKey, err := a.getFunds(iotago.AddressImplicitAccountCreation)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "Failed to create account")
	}

	implicitAccountID := iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID)
	log.Infof("Implicit account created, outputID: %s, implicit accountID: %s", implicitAccountOutput.OutputID.ToHex(), implicitAccountID.ToHex())

	if !params.Transition {
		return implicitAccountID, nil
	}

	log.Debugf("Transitioning implicit account with implicitAccountID %s for alias %s to regular account", params.Alias, iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID).ToHex())

	pubKey, isEd25519 := privateKey.Public().(ed25519.PublicKey)
	if !isEd25519 {
		return iotago.EmptyAccountID, ierrors.New("Failed to create account: only Ed25519 keys are supported")
	}

	implicitAccAddr := iotago.ImplicitAccountCreationAddressFromPubKey(pubKey)
	addrKeys := iotago.NewAddressKeysForImplicitAccountCreationAddress(implicitAccAddr, privateKey)
	implicitBlockIssuerKey := iotago.Ed25519PublicKeyHashBlockIssuerKeyFromImplicitAccountCreationAddress(implicitAccAddr)
	blockIssuerKeys := iotago.NewBlockIssuerKeys(implicitBlockIssuerKey)

	return a.transitionImplicitAccount(implicitAccountOutput, implicitAccAddr, addrKeys.Address, blockIssuerKeys, privateKey, params)
}

func (a *AccountWallet) transitionImplicitAccount(
	implicitAccountOutput *models.Output,
	implicitAccAddr *iotago.ImplicitAccountCreationAddress,
	accAddr iotago.Address,
	blockIssuerKeys iotago.BlockIssuerKeys,
	privateKey ed25519.PrivateKey,
	params *CreateAccountParams,
) (iotago.AccountID, error) {
	requiredMana, err := a.estimateMinimumRequiredMana(1, 0, true)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to estimate number of faucet requests to cover minimum required mana")
	}
	log.Debugf("Mana required for account creation: %d, requesting additional mana from the faucet", requiredMana)
	additionalBasicInputs, err := a.RequestManaFromTheFaucet(requiredMana, implicitAccAddr)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request mana from the faucet")
	}
	if len(additionalBasicInputs) > 0 {
		log.Debugf("successfully requested %d new outputs from the faucet", len(additionalBasicInputs))
	}
	balance := implicitAccountOutput.Balance + utils.SumOutputsBalance(additionalBasicInputs)
	// transition from implicit to regular account
	accountOutput := builder.NewAccountOutputBuilder(accAddr, balance).
		Mana(implicitAccountOutput.OutputStruct.StoredMana()).
		AccountID(iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID)).
		BlockIssuer(blockIssuerKeys, iotago.MaxSlotIndex).MustBuild()

	log.Infof("Created account %s with %d tokens\n", accountOutput.AccountID.ToHex(), accountOutput.Amount)
	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(a.client.Client(), a.faucet.account.ID())
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}
	inputs := append([]*models.Output{implicitAccountOutput}, additionalBasicInputs...)
	implicitAccountID := iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID)
	txBuilder := a.createTransactionBuilder(inputs, implicitAccAddr, accountOutput)
	commitmentID, _ := issuerResp.Commitment.ID()
	txBuilder.AddContextInput(&iotago.CommitmentInput{CommitmentID: commitmentID})

	a.logMissingMana(txBuilder, congestionResp.ReferenceManaCost, implicitAccountID)
	txBuilder.AllotRequiredManaAndStoreRemainingManaInOutput(txBuilder.CreationSlot(), congestionResp.ReferenceManaCost, implicitAccountID, 0)
	addrSigner, err := a.getAddrSignerForIndexes(inputs...)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to get address signer")
	}

	signedTx, err := txBuilder.Build(addrSigner)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to build tx")
	}

	accountID := a.registerAccount(params.Alias, implicitAccountOutput.OutputID, implicitAccountOutput.AddressIndex, privateKey)
	accountHandler := mock.NewEd25519Account(accountID, privateKey)
	blkID, err := a.PostWithBlock(a.client, signedTx, accountHandler, congestionResp, issuerResp, version)
	if err != nil {
		log.Errorf("Failed to post account with block: %s", err)

		return iotago.EmptyAccountID, ierrors.Wrap(err, "failed to post transaction")
	}
	log.Infof("Created account %s with %d tokens, blk ID %s, awaiting the commitment.\n", accountID.ToHex(), accountOutput.Amount, blkID.ToHex())

	err = utils.AwaitCommitment(a.client, blkID.Slot())
	if err != nil {
		return iotago.EmptyAccountID, err
	}

	log.Infof("Slot %d is committed\n", blkID.Slot())

	outputID, account, slot, err := a.client.GetAccountFromIndexer(accountID)
	if err != nil {
		return iotago.EmptyAccountID, ierrors.Wrapf(err, "failed to get account from indexer, even after slot %d is already committed", blkID.Slot())
	}
	log.Infof("Account created, ID: %s, outputID: %s, slot: %d\n", accountID.ToHex(), outputID.ToHex(), slot)
	log.Infof(utils.SprintAccount(account))

	return iotago.EmptyAccountID, nil
}

func (a *AccountWallet) RequestManaFromTheFaucet(minManaAmount iotago.Mana, addr iotago.Address) ([]*models.Output, error) {
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
			faucetOutput, _, err := a.getFunds(iotago.AddressEd25519)
			if err != nil {
				log.Errorf("failed to request funds from the faucet: %v", err)
			}
			outputs = append(outputs, faucetOutput)
		}(requested)
	}
	wg.Wait()

	return outputs, nil
}

func (a *AccountWallet) createAccountWithFaucet(_ *CreateAccountParams) (iotago.AccountID, error) {
	log.Debug("Not implemented yet")
	//faucetOutput := a.getFaucetOutput()
	//
	//accountOutputBuilder := builder.NewAccountOutputBuilder(accAddr, accAddr, implicitAccountOutput.Balance).
	//	Mana(implicitAccountOutput.OutputStruct.StoredMana()).
	//	AccountID(iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID))
	//
	//// adding Block Issuer Feature
	//if !params.NoBIF {
	//	accountOutputBuilder.BlockIssuer(blockIssuerKeys, iotago.MaxSlotIndex).MustBuild()
	//}
	return iotago.EmptyAccountID, nil
}

func (a *AccountWallet) createTransactionBuilder(inputs []*models.Output, address iotago.Address, accountOutput *iotago.AccountOutput) *builder.TransactionBuilder {
	currentTime := time.Now()
	currentSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(currentTime)

	apiForSlot := a.client.APIForSlot(currentSlot)
	txBuilder := builder.NewTransactionBuilder(apiForSlot)
	for _, output := range inputs {
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: address,
			InputID:      output.OutputID,
			Input:        output.OutputStruct,
		})
	}

	txBuilder.AddOutput(accountOutput)
	txBuilder.SetCreationSlot(currentSlot)

	return txBuilder
}

func (a *AccountWallet) estimateMinimumRequiredMana(basicInputCount, basicOutputCount int, accountOutput bool) (iotago.Mana, error) {
	congestionResp, err := a.client.GetCongestion(a.faucet.account.ID())
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to get congestion data for faucet accountID")
	}

	if err != nil {
		return 0, ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}

	txBuilder := builder.NewTransactionBuilder(a.client.APIForSlot(congestionResp.Slot))
	txBuilder.SetCreationSlot(congestionResp.Slot)
	for i := 0; i < basicInputCount; i++ {
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: tpkg.RandEd25519Address(),
			InputID:      iotago.EmptyOutputID,
			Input:        tpkg.RandBasicOutput(iotago.AddressEd25519),
		})
	}
	for i := 0; i < basicOutputCount; i++ {
		txBuilder.AddOutput(tpkg.RandBasicOutput(iotago.AddressEd25519))
	}
	if accountOutput {
		out := builder.NewAccountOutputBuilder(tpkg.RandAccountAddress(), 100).
			Mana(100).
			BlockIssuer(tpkg.RandomBlockIssuerKeysEd25519(1), iotago.MaxSlotIndex).MustBuild()
		txBuilder.AddOutput(out)
	}

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
	log.Debugf("Min required allotted mana: %d\n", minRequiredAllottedMana)
}

func (a *AccountWallet) getAddress(addressType iotago.AddressType) (iotago.DirectUnlockableAddress, ed25519.PrivateKey, uint64) {
	newIndex := a.latestUsedIndex.Inc()
	hdWallet := mock.NewKeyManager(a.seed[:], newIndex)
	privKey, _ := hdWallet.KeyPair()
	receiverAddr := hdWallet.Address(addressType)

	return receiverAddr, privKey, newIndex
}

func (a *AccountWallet) getAddrSignerForIndexes(outputs ...*models.Output) (iotago.AddressSigner, error) {
	var addrKeys []iotago.AddressKeys
	for _, out := range outputs {
		switch out.Address.Type() {
		case iotago.AddressEd25519:
			ed25519Addr := out.Address.(*iotago.Ed25519Address)
			addrKeys = append(addrKeys, iotago.NewAddressKeysForEd25519Address(ed25519Addr, out.PrivKey))
		case iotago.AddressImplicitAccountCreation:
			implicitAccountCreationAddr := out.Address.(*iotago.ImplicitAccountCreationAddress)
			addrKeys = append(addrKeys, iotago.NewAddressKeysForImplicitAccountCreationAddress(implicitAccountCreationAddr, out.PrivKey))
		}
	}

	return iotago.NewInMemoryAddressSigner(addrKeys...), nil
}

func (a *AccountWallet) DestroyAccount(params *DestroyAccountParams) error {
	return a.destroyAccount(params.AccountAlias)
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
