package evilwallet

import (
	"context"
	"sync"

	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// OutputManager keeps track of the output statuses.
type OutputManager struct {
	log.Logger
	connector models.Connector

	wallets         *Wallets
	tempIDWalletMap map[models.TempOutputID]*Wallet

	// stores solid outputs per node
	issuerSolidOutIDMap map[string]map[iotago.OutputID]types.Empty

	syncutils.RWMutex
}

// NewOutputManager creates an OutputManager instance. All outputs are mapped based on their address, so address should never be reused.
func NewOutputManager(connector models.Connector, wallets *Wallets, logger log.Logger) *OutputManager {
	return &OutputManager{
		Logger:              logger,
		connector:           connector,
		wallets:             wallets,
		tempIDWalletMap:     make(map[models.TempOutputID]*Wallet),
		issuerSolidOutIDMap: make(map[string]map[iotago.OutputID]types.Empty),
	}
}

// setOutputIDWalletMap sets wallet for the provided outputID.
func (o *OutputManager) setTempIDWalletMap(id models.TempOutputID, wallet *Wallet) {
	o.Lock()
	defer o.Unlock()

	o.tempIDWalletMap[id] = wallet
}

// TempIDWalletMap returns wallet corresponding to the address stored in OutputManager.
func (o *OutputManager) TempIDWalletMap(outputID models.TempOutputID) *Wallet {
	o.RLock()
	defer o.RUnlock()

	return o.tempIDWalletMap[outputID]
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
func (o *OutputManager) createOutputFromAddress(w *Wallet, api iotago.API, addr *iotago.Ed25519Address, outputID iotago.OutputID, outputStruct iotago.Output) *models.Output {
	index := w.AddrIndexMap(addr.String())
	out := lo.PanicOnErr(models.NewOutputWithID(api, outputID, addr, index, nil, outputStruct))

	w.AddUnspentOutput(out.TempID, out)
	o.setTempIDWalletMap(out.TempID, w)

	return out
}

// AddOutput adds existing output from wallet w to the OutputManager.
func (o *OutputManager) AddOutput(api iotago.API, w *Wallet, output iotago.Output) *models.Output {
	addr := output.UnlockConditionSet().Address().Address
	idx := w.AddrIndexMap(addr.String())

	out := lo.PanicOnErr(models.NewOutputWithEmptyID(api, addr, idx, nil, output))

	w.AddUnspentOutput(out.TempID, out)
	o.setTempIDWalletMap(out.TempID, w)

	return out
}

// GetOutput returns the Output for the given address.
// Firstly checks if output can be retrieved by outputManager from wallet, if not does an API call.
func (o *OutputManager) GetOutput(ctx context.Context, id models.TempOutputID, outputID iotago.OutputID) (output *models.Output) {
	output = o.getOutputFromWallet(id)

	// get output info via web api
	if output == nil {
		if outputID == iotago.EmptyOutputID {
			return nil
		}
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

func (o *OutputManager) getOutputFromWallet(id models.TempOutputID) (output *models.Output) {
	o.RLock()
	defer o.RUnlock()

	w, ok := o.tempIDWalletMap[id]
	if ok {
		output = w.UnspentOutput(id)
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
	o.LogDebugf("Awaiting confirmation of %d transactions", len(txIDs))

	for _, txID := range txIDs {
		wg.Add(1)
		go func(ctx context.Context, txID iotago.TransactionID, clt models.Client) {
			defer wg.Done()
			semaphore <- true
			defer func() {
				<-semaphore
			}()

			err := utils.AwaitBlockWithTransactionToBeAccepted(ctx, o.Logger, clt, txID)
			txLeft.Dec()
			if err != nil {
				o.LogErrorf("Error awaiting transaction %s to be accepted, tx left: %d, err: %s, ", txID.String(), txLeft.Load(), err)

				return
			}

			o.LogDebugf("Tx %s accepted, tx left: %d", txID.ToHex(), txLeft.Load())

		}(ctx, txID, o.connector.GetClient())
	}
	wg.Wait()
}
