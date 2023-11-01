package accountwallet

import (
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/iotaledger/evil-tools/models"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

// CreateAccount creates an implicit account and immediately transition it to a regular account.
func (a *AccountWallet) CreateAccount(params *CreateAccountParams) (iotago.AccountID, error) {
	if params.Implicit {
		return a.createAccountImplicitly(params)
	}

	return a.createAccountWithFaucet(params)
}

func (a *AccountWallet) createAccountImplicitly(params *CreateAccountParams) (iotago.AccountID, error) {
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
	_ ed25519.PrivateKey,
	params *CreateAccountParams,
) (iotago.AccountID, error) {
	// transition from implicit to regular account
	accountOutput := builder.NewAccountOutputBuilder(accAddr, accAddr, implicitAccountOutput.Balance).
		Mana(implicitAccountOutput.OutputStruct.StoredMana()).
		AccountID(iotago.AccountIDFromOutputID(implicitAccountOutput.OutputID)).
		BlockIssuer(blockIssuerKeys, iotago.MaxSlotIndex).MustBuild()

	log.Infof("Created account %s with %d tokens\n", accountOutput.AccountID.ToHex())
	txBuilder := a.createTransactionBuilder(implicitAccountOutput, implicitAccAddr, accountOutput)

	// TODO get congestionResponse from API
	var rmc iotago.Mana
	var implicitAccountID iotago.AccountID
	txBuilder.AllotRequiredManaAndStoreRemainingManaInOutput(txBuilder.CreationSlot(), rmc, implicitAccountID, 0)

	//signedTx, err := txBuilder.Build(a.genesisHdWallet.AddressSigner())

	//accountID := a.registerAccount(params.Alias, implicitAccountOutput.OutputID, a.latestUsedIndex, privateKey)

	//fmt.Printf("Created account %s with %d tokens\n", accountID.ToHex(), params.Amount)
	return iotago.EmptyAccountID, nil
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

func (a *AccountWallet) createTransactionBuilder(input *models.Output, address iotago.Address, accountOutput *iotago.AccountOutput) *builder.TransactionBuilder {
	currentTime := time.Now()
	currentSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(currentTime)

	apiForSlot := a.client.APIForSlot(currentSlot)
	txBuilder := builder.NewTransactionBuilder(apiForSlot)

	txBuilder.AddInput(&builder.TxInput{
		UnlockTarget: address,
		InputID:      input.OutputID,
		Input:        input.OutputStruct,
	})
	txBuilder.AddOutput(accountOutput)
	txBuilder.SetCreationSlot(currentSlot)

	return txBuilder
}

func (a *AccountWallet) getAddress(addressType iotago.AddressType) (iotago.DirectUnlockableAddress, ed25519.PrivateKey, uint64) {
	newIndex := a.latestUsedIndex.Inc()
	hdWallet := mock.NewHDWallet("", a.seed[:], newIndex)
	privKey, _ := hdWallet.KeyPair()
	receiverAddr := hdWallet.Address(addressType)

	return receiverAddr, privKey, newIndex
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
