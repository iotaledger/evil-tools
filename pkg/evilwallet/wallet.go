package evilwallet

import (
	"crypto/ed25519"

	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

// region Wallet ///////////////////////////////////////////////////////////////////////////////////////////////////////

// Wallet is the definition of a wallet.
type Wallet struct {
	ID                walletID
	walletType        WalletType
	unspentOutputs    map[string]*models.Output // maps addr to its unspentOutput
	indexAddrMap      map[uint64]string
	addrIndexMap      map[string]uint64
	inputTransactions map[string]types.Empty
	reuseAddressPool  map[string]types.Empty
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

	wallet := &Wallet{
		walletType:        walletType,
		ID:                -1,
		seed:              tpkg.RandEd25519Seed(),
		unspentOutputs:    make(map[string]*models.Output),
		indexAddrMap:      make(map[uint64]string),
		addrIndexMap:      make(map[string]uint64),
		inputTransactions: make(map[string]types.Empty),
		lastAddrSpent:     *idxSpent,
		lastAddrIdxUsed:   *addrUsed,
		RWMutex:           &syncutils.RWMutex{},
	}

	if walletType == Reuse {
		wallet.reuseAddressPool = make(map[string]types.Empty)
	}

	return wallet
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
	hdWallet := mock.NewKeyManager(w.seed[:], index)
	//nolint:forcetypeassert
	addr := hdWallet.Address(iotago.AddressEd25519).(*iotago.Ed25519Address)
	w.indexAddrMap[index] = addr.String()
	w.addrIndexMap[addr.String()] = index

	return addr
}

// AddressOnIndex returns a new and unused address of a given wallet.
func (w *Wallet) AddressOnIndex(index uint64) *iotago.Ed25519Address {
	w.Lock()
	defer w.Unlock()

	hdWallet := mock.NewKeyManager(w.seed[:], index)
	//nolint:forcetypeassert
	addr := hdWallet.Address(iotago.AddressEd25519).(*iotago.Ed25519Address)

	return addr
}

// UnspentOutput returns the unspent output on the address.
func (w *Wallet) UnspentOutput(addr string) *models.Output {
	w.RLock()
	defer w.RUnlock()

	return w.unspentOutputs[addr]
}

// UnspentOutputs returns all unspent outputs on the wallet.
func (w *Wallet) UnspentOutputs() (outputs map[string]*models.Output) {
	w.RLock()
	defer w.RUnlock()
	outputs = make(map[string]*models.Output)
	for addr, outs := range w.unspentOutputs {
		outputs[addr] = outs
	}

	return outputs
}

// IndexAddrMap returns the address for the index specified.
func (w *Wallet) IndexAddrMap(outIndex uint64) string {
	w.RLock()
	defer w.RUnlock()

	return w.indexAddrMap[outIndex]
}

// AddrIndexMap returns the index for the address specified.
func (w *Wallet) AddrIndexMap(address string) uint64 {
	w.RLock()
	defer w.RUnlock()

	return w.addrIndexMap[address]
}

// AddUnspentOutput adds an unspentOutput of a given wallet.
func (w *Wallet) AddUnspentOutput(output *models.Output) {
	w.Lock()
	defer w.Unlock()

	w.unspentOutputs[output.Address.String()] = output

	if w.walletType == Reuse {
		w.reuseAddressPool[output.Address.String()] = types.Void
	}
}

// UnspentOutputBalance returns the balance on the unspent output sitting on the address specified.
func (w *Wallet) UnspentOutputBalance(addr string) iotago.BaseToken {
	w.RLock()
	defer w.RUnlock()

	total := iotago.BaseToken(0)
	if out, ok := w.unspentOutputs[addr]; ok {
		total += out.Balance
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
		left = len(w.reuseAddressPool)
	default:
		left = int(w.lastAddrIdxUsed.Load() - w.lastAddrSpent.Load())
	}

	return
}

// AddReuseAddress adds address to the reuse ready outputs' addresses pool for a Reuse wallet.
func (w *Wallet) AddReuseAddress(addr string) {
	w.Lock()
	defer w.Unlock()

	if w.walletType == Reuse {
		w.reuseAddressPool[addr] = types.Void
	}
}

// GetReuseAddress get random address from reuse addresses reuseOutputsAddresses pool. Address is removed from the pool after selecting.
func (w *Wallet) GetReuseAddress() string {
	w.Lock()
	defer w.Unlock()

	if w.walletType == Reuse {
		if len(w.reuseAddressPool) > 0 {
			for addr := range w.reuseAddressPool {
				delete(w.reuseAddressPool, addr)
				return addr
			}
		}
	}

	return ""
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
			addr := w.IndexAddrMap(uint64(idx))
			outs := w.UnspentOutput(addr)

			return outs
		}
	}

	return nil
}

func (w *Wallet) KeyPair(index uint64) (ed25519.PrivateKey, ed25519.PublicKey) {
	w.RLock()
	defer w.RUnlock()
	hdWallet := mock.NewKeyManager(w.seed[:], index)

	return hdWallet.KeyPair()
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
