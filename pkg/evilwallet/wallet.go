package evilwallet

import (
	"crypto/ed25519"

	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/iotaledger/iota.go/v4/wallet"
)

// region Wallet ///////////////////////////////////////////////////////////////////////////////////////////////////////

// Wallet is the definition of a wallet.
type Wallet struct {
	ID                walletID
	walletType        WalletType
	unspentOutputs    map[models.TempOutputID]*models.Output // maps addr to its unspentOutput
	indexTempIDMap    map[uint64]models.TempOutputID
	addrIndexMap      map[string]uint64
	inputTransactions map[string]types.Empty
	reuseTempIDPool   map[models.TempOutputID]types.Empty
	seed              [32]byte

	lastAddrIdxUsed atomic.Int64 // used during filling in wallet with new outputs
	lastAddrSpent   atomic.Int64 // used during spamming with outputs one by one

	*syncutils.RWMutex
}

// NewWallet creates a wallet of a given type.
func NewWallet(wType ...WalletType) *Wallet {
	walletType := Other
	if len(wType) > 0 {
		walletType = wType[0]
	}
	idxSpent := atomic.NewInt64(-1)
	addrUsed := atomic.NewInt64(-1)

	w := &Wallet{
		walletType:        walletType,
		ID:                -1,
		seed:              tpkg.RandEd25519Seed(),
		unspentOutputs:    make(map[models.TempOutputID]*models.Output),
		indexTempIDMap:    make(map[uint64]models.TempOutputID),
		addrIndexMap:      make(map[string]uint64),
		inputTransactions: make(map[string]types.Empty),
		lastAddrSpent:     *idxSpent,
		lastAddrIdxUsed:   *addrUsed,
		RWMutex:           &syncutils.RWMutex{},
	}

	if walletType == Reuse {
		w.reuseTempIDPool = make(map[models.TempOutputID]types.Empty)
	}

	return w
}

// Type returns the wallet type.
func (w *Wallet) Type() WalletType {
	return w.walletType
}

// Address returns a new and unused address of a given wallet.
func (w *Wallet) Address() *iotago.Ed25519Address {
	w.Lock()
	defer w.Unlock()

	index := uint64(w.lastAddrIdxUsed.Add(1))
	keyManager := lo.PanicOnErr(wallet.NewKeyManager(w.seed[:], index))
	//nolint:forcetypeassert
	addr := keyManager.Address(iotago.AddressEd25519).(*iotago.Ed25519Address)
	w.addrIndexMap[addr.String()] = index

	return addr
}

// AddressOnIndex returns a new and unused address of a given wallet.
func (w *Wallet) AddressOnIndex(index uint64) *iotago.Ed25519Address {
	w.Lock()
	defer w.Unlock()

	keyManager := lo.PanicOnErr(wallet.NewKeyManager(w.seed[:], index))
	//nolint:forcetypeassert
	addr := keyManager.Address(iotago.AddressEd25519).(*iotago.Ed25519Address)

	return addr
}

// UnspentOutput returns the unspent output on the address.
func (w *Wallet) UnspentOutput(id models.TempOutputID) *models.Output {
	w.RLock()
	defer w.RUnlock()

	return w.unspentOutputs[id]
}

// UnspentOutputs returns all unspent outputs on the wallet.
func (w *Wallet) UnspentOutputs() (outputs map[models.TempOutputID]*models.Output) {
	w.RLock()
	defer w.RUnlock()
	outputs = make(map[models.TempOutputID]*models.Output)
	for addr, outs := range w.unspentOutputs {
		outputs[addr] = outs
	}

	return outputs
}

// IndexTempIDMap returns the address for the index specified.
func (w *Wallet) IndexTempIDMap(outIndex uint64) models.TempOutputID {
	w.RLock()
	defer w.RUnlock()

	return w.indexTempIDMap[outIndex]
}

// AddrIndexMap returns the index for the address specified.
func (w *Wallet) AddrIndexMap(address string) uint64 {
	w.RLock()
	defer w.RUnlock()

	return w.addrIndexMap[address]
}

// AddUnspentOutput adds an unspentOutput of a given wallet.
func (w *Wallet) AddUnspentOutput(id models.TempOutputID, output *models.Output) {
	w.Lock()
	defer w.Unlock()

	w.unspentOutputs[id] = output
	w.indexTempIDMap[output.AddressIndex] = id

	if w.walletType == Reuse {
		w.reuseTempIDPool[id] = types.Void
	}
}

// UnspentOutputBalance returns the balance on the unspent output sitting on the address specified.
func (w *Wallet) UnspentOutputBalance(id models.TempOutputID) iotago.BaseToken {
	w.RLock()
	defer w.RUnlock()

	total := iotago.BaseToken(0)
	if out, ok := w.unspentOutputs[id]; ok {
		total += out.OutputStruct.BaseTokenAmount()
	}

	return total
}

// IsEmpty returns true if the wallet is empty.
func (w *Wallet) IsEmpty() (empty bool) {
	return w.UnspentOutputsLeft() <= 0
}

// UnspentOutputsLeft returns how many unused outputs are available in wallet.
func (w *Wallet) UnspentOutputsLeft() (left int) {
	w.RLock()
	defer w.RUnlock()

	switch w.walletType {
	case Reuse:
		left = len(w.reuseTempIDPool)
	default:
		left = int(w.lastAddrIdxUsed.Load() - w.lastAddrSpent.Load())
	}

	return
}

// AddReuseAddress adds address to the reuse ready outputs' addresses pool for a Reuse wallet.
func (w *Wallet) AddReuseAddress(id models.TempOutputID) {
	w.Lock()
	defer w.Unlock()

	if w.walletType == Reuse {
		w.reuseTempIDPool[id] = types.Void
	}
}

// GetReuseAddress get random address from reuse addresses reuseOutputsAddresses pool. Address is removed from the pool after selecting.
func (w *Wallet) GetReuseAddress() models.TempOutputID {
	w.Lock()
	defer w.Unlock()

	if w.walletType == Reuse {
		if len(w.reuseTempIDPool) > 0 {
			for id := range w.reuseTempIDPool {
				delete(w.reuseTempIDPool, id)
				return id
			}
		}
	}

	return models.EmptyTempOutputID
}

// GetUnspentOutput returns an unspent output on the oldest address ordered by index.
func (w *Wallet) GetUnspentOutput() *models.Output {
	switch w.walletType {
	case Reuse:
		addr := w.GetReuseAddress()
		return w.UnspentOutput(addr)
	default:
		if w.lastAddrSpent.Load() < w.lastAddrIdxUsed.Load() {
			idx := w.lastAddrSpent.Inc()
			addr := w.IndexTempIDMap(uint64(idx))
			outs := w.UnspentOutput(addr)

			return outs
		}
	}

	return nil
}

func (w *Wallet) KeyPair(index uint64) (ed25519.PrivateKey, ed25519.PublicKey) {
	w.RLock()
	defer w.RUnlock()

	keyManger := lo.PanicOnErr(wallet.NewKeyManager(w.seed[:], index))

	return keyManger.KeyPair()
}

// UpdateUnspentOutputID updates the unspent output on the address specified.
// func (w *Wallet) UpdateUnspentOutputID(addr string, outputID utxo.OutputID) error {
// 	w.RLock()
// 	walletOutput, ok := w.unspentOutputs[addr]
// 	w.RUnlock()
// 	if !ok {
// 		return errors.Errorf("could not find unspent output under provided address in the wallet, outID:%s, addr: %s", outputID.Base58(), addr)
// 	}
// 	w.Lock()
// 	walletOutput.OutputID = outputID
// 	w.Unlock()
// 	return nil
// }

// UnspentOutputsLength returns the number of unspent outputs on the wallet.
func (w *Wallet) UnspentOutputsLength() int {
	return len(w.unspentOutputs)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////
