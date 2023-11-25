package evilwallet

import (
	"context"
	"sync"

	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// OutputManager keeps track of the output statuses.
type OutputManager struct {
	connector models.Connector

	wallets       *Wallets
	addrWalletMap map[string]*Wallet

	// stores solid outputs per node
	issuerSolidOutIDMap map[string]map[iotago.OutputID]types.Empty

	log *logger.Logger

	syncutils.RWMutex
}

// NewOutputManager creates an OutputManager instance. All outputs are mapped based on their address, so address should never be reused
func NewOutputManager(connector models.Connector, wallets *Wallets, log *logger.Logger) *OutputManager {
	return &OutputManager{
		connector:           connector,
		wallets:             wallets,
		addrWalletMap:       make(map[string]*Wallet),
		issuerSolidOutIDMap: make(map[string]map[iotago.OutputID]types.Empty),
		log:                 log,
	}
}

// setOutputIDWalletMap sets wallet for the provided outputID.
func (o *OutputManager) setAddrWalletMap(address string, wallet *Wallet) {
	o.Lock()
	defer o.Unlock()

	o.addrWalletMap[address] = wallet
}

// AddressWalletMap returns wallet corresponding to the address stored in OutputManager.
func (o *OutputManager) AddressWalletMap(outputID string) *Wallet {
	o.RLock()
	defer o.RUnlock()

	return o.addrWalletMap[outputID]
}

// SetOutputIDSolidForIssuer sets solid flag for the provided outputID and issuer.
func (o *OutputManager) SetOutputIDSolidForIssuer(outputID iotago.OutputID, issuer string) {
	o.Lock()
	defer o.Unlock()

	if _, ok := o.issuerSolidOutIDMap[issuer]; !ok {
		o.issuerSolidOutIDMap[issuer] = make(map[iotago.OutputID]types.Empty)
	}
	o.issuerSolidOutIDMap[issuer][outputID] = types.Void
}

// IssuerSolidOutIDMap checks whether output was marked as solid for a given node.
func (o *OutputManager) IssuerSolidOutIDMap(issuer string, outputID iotago.OutputID) (isSolid bool) {
	o.RLock()
	defer o.RUnlock()

	if solidOutputs, ok := o.issuerSolidOutIDMap[issuer]; ok {
		if _, isSolid = solidOutputs[outputID]; isSolid {
			return
		}
	}

	return
}

// Track the confirmed statuses of the given outputIDs, it returns true if all of them are confirmed.
func (o *OutputManager) Track(ctx context.Context, outputIDs ...iotago.OutputID) (allConfirmed bool) {
	var (
		wg                     sync.WaitGroup
		unconfirmedOutputFound atomic.Bool
	)

	for _, ID := range outputIDs {
		wg.Add(1)

		go func(id iotago.OutputID, clt models.Client) {
			defer wg.Done()

			if err := utils.AwaitOutputToBeAccepted(ctx, clt, id); err == nil {
				unconfirmedOutputFound.Store(true)
			}
		}(ID, o.connector.GetClient())
	}
	wg.Wait()

	return !unconfirmedOutputFound.Load()
}

// createOutputFromAddress creates output, retrieves outputID, and adds it to the wallet.
// Provided address should be generated from provided wallet. Considers only first output found on address.
func (o *OutputManager) createOutputFromAddress(w *Wallet, addr *iotago.Ed25519Address, outputID iotago.OutputID, outputStruct iotago.Output) *models.Output {
	index := w.AddrIndexMap(addr.String())
	out := &models.Output{
		Address:      addr,
		AddressIndex: index,
		OutputID:     outputID,
		OutputStruct: outputStruct,
	}
	w.AddUnspentOutput(out)
	o.setAddrWalletMap(out.Address.String(), w)

	return out
}

// AddOutput adds existing output from wallet w to the OutputManager.
func (o *OutputManager) AddOutput(ctx context.Context, w *Wallet, output iotago.Output) *models.Output {
	addr := output.UnlockConditionSet().Address().Address
	idx := w.AddrIndexMap(addr.String())
	out := &models.Output{
		Address:      addr,
		AddressIndex: idx,
		OutputStruct: output,
	}

	if w.walletType == Reuse {
		go func(clt models.Client, wallet *Wallet, outputID iotago.OutputID) {
			// Reuse wallet should only keep accepted outputs
			err := utils.AwaitOutputToBeAccepted(ctx, clt, outputID)
			if err != nil {
				o.log.Errorf("Output %s not accepted in time: %v", outputID.String(), err)

				return
			}

			w.AddUnspentOutput(out)
			o.setAddrWalletMap(out.Address.String(), wallet)
		}(o.connector.GetClient(), w, out.OutputID)

		return out
	}

	w.AddUnspentOutput(out)
	o.setAddrWalletMap(out.Address.String(), w)

	return out
}

// GetOutput returns the Output for the given address.
// Firstly checks if output can be retrieved by outputManager from wallet, if not does an API call.
func (o *OutputManager) GetOutput(ctx context.Context, addr string, outputID iotago.OutputID) (output *models.Output) {
	output = o.getOutputFromWallet(addr)

	// get output info via web api
	if output == nil {
		clt := o.connector.GetClient()
		out := clt.GetOutput(ctx, outputID)
		if out == nil {
			return nil
		}

		basicOutput, isBasic := out.(*iotago.BasicOutput)
		if !isBasic {
			return nil
		}

		output = &models.Output{
			OutputID:     outputID,
			Address:      basicOutput.UnlockConditionSet().Address().Address,
			OutputStruct: basicOutput,
		}
	}

	return output
}

func (o *OutputManager) getOutputFromWallet(addr string) (output *models.Output) {
	o.RLock()
	defer o.RUnlock()

	w, ok := o.addrWalletMap[addr]
	if ok {
		output = w.UnspentOutput(addr)
	}

	return
}

// AwaitWalletOutputsToBeConfirmed awaits for all outputs in the wallet are confirmed.
func (o *OutputManager) AwaitWalletOutputsToBeConfirmed(ctx context.Context, wallet *Wallet) {
	wg := sync.WaitGroup{}
	for _, output := range wallet.UnspentOutputs() {
		wg.Add(1)
		if output == nil {
			continue
		}

		var outs iotago.OutputIDs
		outs = append(outs, output.OutputID)

		go func(outs iotago.OutputIDs) {
			defer wg.Done()

			o.Track(ctx, outs...)
		}(outs)
	}
	wg.Wait()
}

// AwaitTransactionsAcceptance awaits for transaction confirmation and updates wallet with outputIDs.
func (o *OutputManager) AwaitTransactionsAcceptance(ctx context.Context, txIDs ...iotago.TransactionID) {
	wg := sync.WaitGroup{}
	semaphore := make(chan bool, 10)
	txLeft := atomic.NewInt64(int64(len(txIDs)))
	o.log.Debugf("Awaiting confirmation of %d transactions", len(txIDs))

	for _, txID := range txIDs {
		wg.Add(1)
		go func(ctx context.Context, txID iotago.TransactionID, clt models.Client) {
			defer wg.Done()
			semaphore <- true
			defer func() {
				<-semaphore
			}()

			err := utils.AwaitBlockWithTransactionToBeAccepted(ctx, clt, txID)
			txLeft.Dec()
			if err != nil {
				o.log.Errorf("Error awaiting transaction %s to be accepted, tx left: %d, err: %s, ", txID.String(), txLeft.Load(), err)

				return
			}

			o.log.Debugf("Tx %s accepted, tx left: %d", txID.ToHex(), txLeft.Load())

		}(ctx, txID, o.connector.GetClient())
	}
	wg.Wait()
}
