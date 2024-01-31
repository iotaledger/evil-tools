package accountmanager

import (
	"crypto/ed25519"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/iotaledger/iota.go/v4/wallet"
)

type Wallet struct {
	Alias           string   `serix:"alias,lenPrefix=uint8"`
	Seed            [32]byte `serix:"seed"`
	LatestUsedIndex uint32   `serix:"LatestUsedIndex"`
}

func newAccountWallet(alias string) *Wallet {
	return &Wallet{
		Alias: alias,
		Seed:  tpkg.RandEd25519Seed(),
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

func (w *Wallet) GetAddrSignerForIndexes(outputs ...*models.OutputData) (iotago.AddressSigner, error) {
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

func (w *Wallet) getAddress(addressType iotago.AddressType) (iotago.DirectUnlockableAddress, ed25519.PrivateKey, uint32) {
	w.LatestUsedIndex++
	newIndex := w.LatestUsedIndex
	keyManager := lo.PanicOnErr(wallet.NewKeyManager(w.Seed[:], BIP32PathForIndex(newIndex)))
	privateKey, _ := keyManager.KeyPair()
	receiverAddr := keyManager.Address(addressType)

	return receiverAddr, privateKey, newIndex
}

func (w *Wallet) getPrivateKeyForIndex(index uint32) ed25519.PrivateKey {
	keyManager := lo.PanicOnErr(wallet.NewKeyManager(w.Seed[:], BIP32PathForIndex(index)))
	privateKey, _ := keyManager.KeyPair()

	return privateKey
}

func (w *Wallet) createOutputDataForIndex(outputID iotago.OutputID, index uint32, outputStruct iotago.Output) *models.OutputData {
	privateKey := w.getPrivateKeyForIndex(index)

	return &models.OutputData{
		OutputID:     outputID,
		Address:      outputStruct.UnlockConditionSet().Address().Address,
		AddressIndex: index,
		PrivateKey:   privateKey,
		OutputStruct: outputStruct,
	}
}
