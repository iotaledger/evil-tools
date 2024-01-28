package accountmanager

import (
	"crypto"
	"crypto/ed25519"

	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/pkg/models"
	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/iotaledger/iota.go/v4/wallet"
)

type Wallet struct {
	alias           string        `serix:"alias,lenPrefix=uint8"`
	seed            [32]byte      `serix:"seed"`
	latestUsedIndex atomic.Uint32 `serix:"latestUsedIndex"`
}

func newAccountWallet(alias string) *Wallet {
	return &Wallet{
		alias: alias,
		seed:  tpkg.RandEd25519Seed(),
	}
}

func (m *Manager) newAccountWallet(alias string) *Wallet {
	accountWallet := newAccountWallet(alias)

	m.wallets[alias] = accountWallet

	return accountWallet
}

func (m *Manager) getOrCreateWallet(alias string) *Wallet {
	m.Lock()
	defer m.Unlock()

	w, exists := m.wallets[alias]
	if !exists {
		return m.newAccountWallet(alias)
	}

	return w
}

func (a *Wallet) GetAddrSignerForIndexes(outputs ...*models.OutputData) (iotago.AddressSigner, error) {
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

func (a *Wallet) getAccountPublicKeys(pubKey crypto.PublicKey) (iotago.BlockIssuerKeys, error) {
	ed25519PubKey, isEd25519 := pubKey.(ed25519.PublicKey)
	if !isEd25519 {
		return nil, ierrors.New("Failed to create account: only Ed25519 keys are supported")
	}

	blockIssuerKeys := iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(hiveEd25519.PublicKey(ed25519PubKey)))

	return blockIssuerKeys, nil

}

func (a *Wallet) getAddress(addressType iotago.AddressType) (iotago.DirectUnlockableAddress, ed25519.PrivateKey, uint32) {
	newIndex := a.latestUsedIndex.Inc()
	keyManager := lo.PanicOnErr(wallet.NewKeyManager(a.seed[:], BIP32PathForIndex(newIndex)))
	privateKey, _ := keyManager.KeyPair()
	receiverAddr := keyManager.Address(addressType)

	return receiverAddr, privateKey, newIndex
}

func (a *Wallet) getPrivateKeyForIndex(index uint32) ed25519.PrivateKey {
	keyManager := lo.PanicOnErr(wallet.NewKeyManager(a.seed[:], BIP32PathForIndex(index)))
	privateKey, _ := keyManager.KeyPair()

	return privateKey
}

func (a *Wallet) createOutputDataForIndex(outputID iotago.OutputID, index uint32, outputStruct iotago.Output) *models.OutputData {
	privateKey := a.getPrivateKeyForIndex(index)

	return &models.OutputData{
		OutputID:     outputID,
		Address:      outputStruct.UnlockConditionSet().Address().Address,
		AddressIndex: index,
		PrivateKey:   privateKey,
		OutputStruct: outputStruct,
	}
}
