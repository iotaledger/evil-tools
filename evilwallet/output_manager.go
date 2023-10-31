package evilwallet

import (
	"context"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/models"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

const (
	awaitOutputToBeConfirmed = 10 * time.Second
)

// OutputManager keeps track of the output statuses.
type OutputManager struct {
	connector models.Connector

	wallets           *Wallets
	outputIDWalletMap map[string]*Wallet
	outputIDAddrMap   map[string]string
	// stores solid outputs per node
	issuerSolidOutIDMap map[string]map[iotago.OutputID]types.Empty

	log *logger.Logger

	syncutils.RWMutex
}

// NewOutputManager creates an OutputManager instance.
func NewOutputManager(connector models.Connector, wallets *Wallets, log *logger.Logger) *OutputManager {
	return &OutputManager{
		connector:           connector,
		wallets:             wallets,
		outputIDWalletMap:   make(map[string]*Wallet),
		outputIDAddrMap:     make(map[string]string),
		issuerSolidOutIDMap: make(map[string]map[iotago.OutputID]types.Empty),
		log:                 log,
	}
}

// setOutputIDWalletMap sets wallet for the provided outputID.
func (o *OutputManager) setOutputIDWalletMap(outputID string, wallet *Wallet) {
	o.Lock()
	defer o.Unlock()

	o.outputIDWalletMap[outputID] = wallet
}

// setOutputIDAddrMap sets address for the provided outputID.
func (o *OutputManager) setOutputIDAddrMap(outputID string, addr string) {
	o.Lock()
	defer o.Unlock()

	o.outputIDAddrMap[outputID] = addr
}

// OutputIDWalletMap returns wallet corresponding to the outputID stored in OutputManager.
func (o *OutputManager) OutputIDWalletMap(outputID string) *Wallet {
	o.RLock()
	defer o.RUnlock()

	return o.outputIDWalletMap[outputID]
}

// OutputIDAddrMap returns address corresponding to the outputID stored in OutputManager.
func (o *OutputManager) OutputIDAddrMap(outputID string) (addr string) {
	o.RLock()
	defer o.RUnlock()

	addr = o.outputIDAddrMap[outputID]

	return
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
func (o *OutputManager) Track(outputIDs ...iotago.OutputID) (allConfirmed bool) {
	var (
		wg                     sync.WaitGroup
		unconfirmedOutputFound atomic.Bool
	)

	for _, ID := range outputIDs {
		wg.Add(1)

		go func(id iotago.OutputID) {
			defer wg.Done()

			if !o.AwaitOutputToBeAccepted(id, awaitOutputToBeConfirmed) {
				unconfirmedOutputFound.Store(true)
			}
		}(ID)
	}
	wg.Wait()

	return !unconfirmedOutputFound.Load()
}

// createOutputFromAddress creates output, retrieves outputID, and adds it to the wallet.
// Provided address should be generated from provided wallet. Considers only first output found on address.
func (o *OutputManager) createOutputFromAddress(w *Wallet, addr *iotago.Ed25519Address, balance iotago.BaseToken, outputID iotago.OutputID, outputStruct iotago.Output) *models.Output {
	index := w.AddrIndexMap(addr.String())
	out := &models.Output{
		Address:      addr,
		AddressIndex: index,
		OutputID:     outputID,
		Balance:      balance,
		OutputStruct: outputStruct,
	}
	w.AddUnspentOutput(out)
	o.setOutputIDWalletMap(outputID.ToHex(), w)
	o.setOutputIDAddrMap(outputID.ToHex(), addr.String())

	return out
}

// AddOutput adds existing output from wallet w to the OutputManager.
func (o *OutputManager) AddOutput(w *Wallet, output *models.Output) *models.Output {
	idx := w.AddrIndexMap(output.Address.String())
	out := &models.Output{
		Address:      output.Address,
		AddressIndex: idx,
		OutputID:     output.OutputID,
		Balance:      output.Balance,
		OutputStruct: output.OutputStruct,
	}
	w.AddUnspentOutput(out)
	o.setOutputIDWalletMap(out.OutputID.ToHex(), w)
	o.setOutputIDAddrMap(out.OutputID.ToHex(), output.Address.String())

	return out
}

// GetOutput returns the Output of the given outputID.
// Firstly checks if output can be retrieved by outputManager from wallet, if not does an API call.
func (o *OutputManager) GetOutput(outputID iotago.OutputID) (output *models.Output) {
	output = o.getOutputFromWallet(outputID)

	// get output info via web api
	if output == nil {
		clt := o.connector.GetClient()
		out := clt.GetOutput(outputID)
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
			Balance:      basicOutput.BaseTokenAmount(),
			OutputStruct: basicOutput,
		}
	}

	return output
}

func (o *OutputManager) getOutputFromWallet(outputID iotago.OutputID) (output *models.Output) {
	o.RLock()
	defer o.RUnlock()
	w, ok := o.outputIDWalletMap[outputID.ToHex()]
	if ok {
		addr := o.outputIDAddrMap[outputID.ToHex()]
		output = w.UnspentOutput(addr)
	}

	return
}

// AwaitWalletOutputsToBeConfirmed awaits for all outputs in the wallet are confirmed.
func (o *OutputManager) AwaitWalletOutputsToBeConfirmed(wallet *Wallet) {
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

			o.Track(outs...)
		}(outs)
	}
	wg.Wait()
}

// AwaitOutputToBeAccepted awaits for output from a provided outputID is accepted. Timeout is waitFor.
// Useful when we have only an address and no transactionID, e.g. faucet funds request.
func (o *OutputManager) AwaitOutputToBeAccepted(outputID iotago.OutputID, waitFor time.Duration) (accepted bool) {
	s := time.Now()
	clt := o.connector.GetClient()
	accepted = false
	for ; time.Since(s) < waitFor; time.Sleep(awaitAcceptationSleep) {
		confirmationState := clt.GetOutputConfirmationState(outputID)
		if confirmationState == "confirmed" {
			accepted = true
			break
		}
	}

	return accepted
}

func (o *OutputManager) AwaitAddressUnspentOutputToBeAccepted(addr *iotago.Ed25519Address, waitFor time.Duration) (outputID iotago.OutputID, output iotago.Output, err error) {
	clt := o.connector.GetIndexerClient()
	indexer, err := clt.Indexer()
	if err != nil {
		return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to get indexer client")
	}

	s := time.Now()
	addrBech := addr.Bech32(clt.CommittedAPI().ProtocolParameters().Bech32HRP())

	for ; time.Since(s) < waitFor; time.Sleep(awaitAcceptationSleep) {
		res, err := indexer.Outputs(context.Background(), &apimodels.BasicOutputsQuery{
			AddressBech32: addrBech,
		})
		if err != nil {
			return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "indexer request failed in request faucet funds")
		}

		for res.Next() {
			unspents, err := res.Outputs(context.TODO())
			if err != nil {
				return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to get faucet unspent outputs")
			}

			if len(unspents) == 0 {
				o.log.Debugf("no unspent outputs found in indexer for address: %s", addrBech)
				break
			}

			return lo.Return1(res.Response.Items.OutputIDs())[0], unspents[0], nil
		}
	}

	return iotago.EmptyOutputID, nil, ierrors.Errorf("no unspent outputs found for address %s due to timeout", addrBech)
}

// AwaitTransactionsAcceptance awaits for transaction confirmation and updates wallet with outputIDs.
func (o *OutputManager) AwaitTransactionsAcceptance(txIDs ...iotago.TransactionID) {
	wg := sync.WaitGroup{}
	semaphore := make(chan bool, 1)
	txLeft := atomic.NewInt64(int64(len(txIDs)))
	o.log.Debugf("Awaiting confirmation of %d transactions", len(txIDs))

	for _, txID := range txIDs {
		wg.Add(1)
		go func(txID iotago.TransactionID) {
			defer wg.Done()
			semaphore <- true
			defer func() {
				<-semaphore
			}()
			err := o.AwaitTransactionToBeAccepted(txID, waitForAcceptance, txLeft)
			txLeft.Dec()
			if err != nil {
				o.log.Errorf("Error awaiting transaction %s to be accepted: %s", txID.String(), err)

				return
			}
		}(txID)
	}
	wg.Wait()
}

// AwaitTransactionToBeAccepted awaits for acceptance of a single transaction.
func (o *OutputManager) AwaitTransactionToBeAccepted(txID iotago.TransactionID, waitFor time.Duration, txLeft *atomic.Int64) error {
	s := time.Now()
	clt := o.connector.GetClient()
	var accepted bool
	for ; time.Since(s) < waitFor; time.Sleep(awaitAcceptationSleep) {
		resp, err := clt.GetBlockState(txID)
		if resp == nil {
			o.log.Debugf("Block state API error: %v", err)

			continue
		}
		if resp.BlockState == apimodels.BlockStateFailed.String() || resp.BlockState == apimodels.BlockStateRejected.String() {
			failureReason, _, _ := apimodels.BlockFailureReasonFromBytes(lo.PanicOnErr(resp.BlockFailureReason.Bytes()))

			return ierrors.Errorf("tx %s failed because block failure: %d", txID, failureReason)
		}

		if resp.TransactionState == apimodels.TransactionStateFailed.String() {
			failureReason, _, _ := apimodels.TransactionFailureReasonFromBytes(lo.PanicOnErr(resp.TransactionFailureReason.Bytes()))

			return ierrors.Errorf("transaction %s failed: %d", txID, failureReason)
		}

		confirmationState := resp.TransactionState

		o.log.Debugf("Tx %s confirmationState: %s, tx left: %d", txID.ToHex(), confirmationState, txLeft.Load())
		if confirmationState == "accepted" || confirmationState == "confirmed" || confirmationState == "finalized" {
			accepted = true
			break
		}
	}
	if !accepted {
		return ierrors.Errorf("transaction %s not accepted in time", txID)
	}

	o.log.Debugf("Transaction %s accepted", txID)

	return nil
}
