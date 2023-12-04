package accountwallet

import (
	"crypto"
	"crypto/ed25519"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/wallet"
)

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

func (a *AccountWallet) GetAddrSignerForIndexes(outputs ...*models.Output) (iotago.AddressSigner, error) {
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

func (a *AccountWallet) getAccountPublicKeys(pubKey crypto.PublicKey) (iotago.BlockIssuerKeys, error) {
	ed25519PubKey, isEd25519 := pubKey.(ed25519.PublicKey)
	if !isEd25519 {
		return nil, ierrors.New("Failed to create account: only Ed25519 keys are supported")
	}

	blockIssuerKeys := iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(ed25519PubKey))

	return blockIssuerKeys, nil

}

func (a *AccountWallet) getAddress(addressType iotago.AddressType) (iotago.DirectUnlockableAddress, ed25519.PrivateKey, uint64) {
	newIndex := a.latestUsedIndex.Inc()
	keyManager := lo.PanicOnErr(wallet.NewKeyManager(a.seed[:], newIndex))
	privateKey, _ := keyManager.KeyPair()
	receiverAddr := keyManager.Address(addressType)

	return receiverAddr, privateKey, newIndex
}
