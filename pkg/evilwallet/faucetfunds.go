package evilwallet

import (
	"context"
	"sync"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	// FaucetRequestSplitNumber defines the number of outputs to split from a faucet request.
	FaucetRequestSplitNumber = 125
)

// RequestFundsFromFaucet requests funds from the faucet, then track the confirmed status of unspent output,
// also register the alias name for the unspent output if provided.
func (e *EvilWallet) RequestFundsFromFaucet(ctx context.Context, options ...FaucetRequestOption) (initWallet *Wallet, err error) {
	initWallet = e.NewWallet(Fresh)
	buildOptions := NewFaucetRequestOptions(options...)

	output, err := e.requestFaucetFunds(ctx, initWallet)
	if err != nil {
		return
	}

	if buildOptions.outputAliasName != "" {
		e.aliasManager.AddInputAlias(output, buildOptions.outputAliasName)
	}

	e.log.Debug("Funds requested successfully")

	return
}

// RequestFreshBigFaucetWallets creates n new wallets, each wallet is created from one faucet request and contains 1000 outputs.
func (e *EvilWallet) RequestFreshBigFaucetWallets(ctx context.Context, numberOfWallets int) bool {
	e.log.Debugf("Requesting %d wallets from faucet", numberOfWallets)
	success := true
	// channel to block the number of concurrent goroutines
	semaphore := make(chan bool, 1)
	wg := sync.WaitGroup{}

	for reqNum := 0; reqNum < numberOfWallets; reqNum++ {
		wg.Add(1)

		// block if full
		semaphore <- true
		go func() {
			defer wg.Done()
			defer func() {
				// release
				<-semaphore
			}()

			err := e.RequestFreshBigFaucetWallet(ctx)
			if err != nil {
				success = false
				e.log.Errorf("Failed to request wallet from faucet: %s", err)

				return
			}
		}()
	}
	wg.Wait()

	e.log.Debugf("Finished requesting %d wallets from faucet, outputs available: %d", numberOfWallets, e.UnspentOutputsLeft(Fresh))

	return success
}

// RequestFreshBigFaucetWallet creates a new wallet and fills the wallet with 1000 outputs created from funds
// requested from the Faucet.
func (e *EvilWallet) RequestFreshBigFaucetWallet(ctx context.Context) error {
	initWallet := NewWallet()
	receiveWallet := e.NewWallet(Fresh)
	_, err := e.requestAndSplitFaucetFunds(ctx, initWallet, receiveWallet)
	if err != nil {
		return ierrors.Wrap(err, "failed to request big funds from faucet")
	}

	e.log.Debug("First level of splitting finished, now split each output once again")
	bigOutputWallet := e.NewWallet(Fresh)
	_, err = e.splitOutputs(ctx, receiveWallet, bigOutputWallet)
	if err != nil {
		return ierrors.Wrap(err, "failed to again split outputs for the big wallet")
	}

	e.wallets.SetWalletReady(bigOutputWallet)

	return nil
}

// RequestFreshFaucetWallet creates a new wallet and fills the wallet with 100 outputs created from funds
// requested from the Faucet.
func (e *EvilWallet) RequestFreshFaucetWallet(ctx context.Context) error {
	initWallet := NewWallet()
	receiveWallet := e.NewWallet(Fresh)
	txID, err := e.requestAndSplitFaucetFunds(ctx, initWallet, receiveWallet)
	if err != nil {
		return ierrors.Wrap(err, "failed to request funds from faucet")
	}

	e.outputManager.AwaitTransactionsAcceptance(ctx, txID)

	e.wallets.SetWalletReady(receiveWallet)

	return err
}

func (e *EvilWallet) requestAndSplitFaucetFunds(ctx context.Context, initWallet, receiveWallet *Wallet) (txID iotago.TransactionID, err error) {
	splitOutput, err := e.requestFaucetFunds(ctx, initWallet)
	if err != nil {
		return iotago.EmptyTransactionID, err
	}

	e.log.Debugf("Faucet funds received, continue spliting output: %s", splitOutput.OutputID.ToHex())

	splitTransactionsID, err := e.splitOutput(ctx, splitOutput, initWallet, receiveWallet)
	if err != nil {
		return iotago.EmptyTransactionID, ierrors.Wrap(err, "failed to split faucet funds")
	}

	return splitTransactionsID, nil
}

func (e *EvilWallet) requestFaucetFunds(ctx context.Context, wallet *Wallet) (output *models.Output, err error) {
	receiveAddr := wallet.AddressOnIndex(0)
	clt := e.connector.GetClient()

	err = clt.RequestFaucetFunds(ctx, receiveAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to request funds from faucet")
	}

	outputID, iotaOutput, err := utils.AwaitAddressUnspentOutputToBeAccepted(ctx, clt, receiveAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to await faucet output acceptance")
	}

	// update wallet with newly created output
	output = e.outputManager.createOutputFromAddress(wallet, receiveAddr, iotaOutput.BaseTokenAmount(), outputID, iotaOutput)

	return output, nil
}

func (e *EvilWallet) splitOutput(ctx context.Context, splitOutput *models.Output, inputWallet, outputWallet *Wallet) (iotago.TransactionID, error) {
	outputs, err := e.createSplitOutputs(splitOutput, outputWallet)
	if err != nil {
		return iotago.EmptyTransactionID, ierrors.Wrapf(err, "failed to create splitted outputs")
	}

	genesisAccount, err := e.accWallet.GetAccount(accountwallet.GenesisAccountAlias)
	if err != nil {
		return iotago.EmptyTransactionID, err
	}

	txData, err := e.CreateTransaction(ctx,
		WithInputs(splitOutput),
		WithOutputs(outputs),
		WithInputWallet(inputWallet),
		WithOutputWallet(outputWallet),
		WithIssuanceStrategy(models.AllotmentStrategyAll, genesisAccount.Account),
	)
	if err != nil {
		return iotago.EmptyTransactionID, err
	}

	_, err = e.PrepareAndPostBlock(ctx, e.connector.GetClient(), txData.Payload, txData.CongestionResponse, genesisAccount.Account)
	if err != nil {
		return iotago.TransactionID{}, err
	}

	if txData.Payload.PayloadType() != iotago.PayloadSignedTransaction {
		return iotago.EmptyTransactionID, ierrors.New("payload type is not signed transaction")
	}

	signedTx, ok := txData.Payload.(*iotago.SignedTransaction)
	if !ok {
		return iotago.EmptyTransactionID, ierrors.New("type assertion error: payload is not a signed transaction")
	}
	txID := lo.PanicOnErr(signedTx.Transaction.ID())
	e.log.Debugf("Splitting output %s finished with tx: %s", splitOutput.OutputID.ToHex(), txID.ToHex())

	return txID, nil
}

// splitOutputs splits all outputs from the provided input wallet, outputs are saved to the outputWallet.
func (e *EvilWallet) splitOutputs(ctx context.Context, inputWallet, outputWallet *Wallet) ([]iotago.TransactionID, error) {
	if inputWallet.IsEmpty() {
		return nil, ierrors.New("failed to split outputs, inputWallet is empty")
	}

	if outputWallet == nil {
		return nil, ierrors.New("failed to split outputs, outputWallet is nil")
	}

	e.log.Debugf("Splitting %d outputs from wallet no %d", len(inputWallet.UnspentOutputs()), inputWallet.ID)
	txIDs := make([]iotago.TransactionID, 0)
	wg := sync.WaitGroup{}
	// split all outputs stored in the input wallet
	for addr := range inputWallet.UnspentOutputs() {
		wg.Add(1)

		go func(addr string) {
			defer wg.Done()

			input := inputWallet.UnspentOutput(addr)
			txID, err := e.splitOutput(ctx, input, inputWallet, outputWallet)
			if err != nil {
				e.log.Errorf("Failed to split output %s: %s", input.OutputID.ToHex(), err)

				return
			}
			txIDs = append(txIDs, txID)
		}(addr)
	}
	wg.Wait()
	e.log.Debug("All blocks with splitting transactions were posted")

	e.outputManager.AwaitTransactionsAcceptance(ctx, txIDs...)

	return txIDs, nil
}

func (e *EvilWallet) createSplitOutputs(input *models.Output, receiveWallet *Wallet) ([]*OutputOption, error) {
	totalAmount := input.Balance
	splitNumber := FaucetRequestSplitNumber
	minDeposit := e.minOutputStorageDeposit

	// make sure the amount of output covers the min deposit
	amountPerOutput, err := safemath.SafeDiv(totalAmount, iotago.BaseToken(splitNumber))
	if err != nil {
		e.log.Errorf("Failed to calculate amount per output, total amount %d, splitted amount %d: %s", totalAmount, 128, err)
	}

	if amountPerOutput < minDeposit {
		outputsNum, err := safemath.SafeDiv(totalAmount, minDeposit)
		if err != nil {
			e.log.Errorf("Failed to calculate split number, total amount %d, splitted amount %d: %s", totalAmount, minDeposit, err)

			return nil, ierrors.Wrapf(err, "failed to calculate split number")
		}

		splitNumber = int(outputsNum)
	}

	balances := utils.SplitBalanceEqually(splitNumber, input.Balance)
	outputs := make([]*OutputOption, splitNumber)
	for i, bal := range balances {
		outputs[i] = &OutputOption{amount: bal, address: receiveWallet.Address(), outputType: iotago.OutputBasic}
	}

	return outputs, nil
}
