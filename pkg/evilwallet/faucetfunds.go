package evilwallet

import (
	"context"
	"sync"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

// RequestFundsFromFaucet requests funds from the faucet, then track the confirmed status of unspent output,
// also register the alias name for the unspent output if provided.
func (e *EvilWallet) RequestFundsFromFaucet(ctx context.Context) (initWallet *Wallet, err error) {
	initWallet = e.NewWallet(Fresh)

	_, err = e.requestFaucetFunds(ctx, initWallet)
	if err != nil {
		return
	}

	e.LogDebug("Funds requested successfully")

	return
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

	e.LogDebug("First level of splitting finished, now split each output once again")
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

	e.LogDebugf("Faucet funds received, continue spliting output: %s", splitOutput.OutputID.ToHex())

	splitTransactionsID, err := e.splitOutput(ctx, splitOutput, initWallet, receiveWallet)
	if err != nil {
		return iotago.EmptyTransactionID, ierrors.Wrap(err, "failed to split faucet funds")
	}

	return splitTransactionsID, nil
}

func (e *EvilWallet) requestFaucetFunds(ctx context.Context, wallet *Wallet) (output *models.OutputData, err error) {
	receiveAddr := wallet.AddressOnIndex(0)
	clt := e.connector.GetClient()

	err = clt.RequestFaucetFunds(ctx, receiveAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to request funds from faucet")
	}

	outputID, iotaOutput, err := utils.AwaitAddressUnspentOutputToBeAccepted(ctx, e.Logger, clt, receiveAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to await faucet output acceptance")
	}

	// update wallet with newly created output
	output = e.outputManager.createOutputFromAddress(wallet, e.accManager.API, receiveAddr, outputID, iotaOutput)

	return output, nil
}

func (e *EvilWallet) splitOutput(ctx context.Context, splitOutput *models.OutputData, inputWallet, outputWallet *Wallet) (iotago.TransactionID, error) {
	outputs, err := e.createSplitOutputs(splitOutput, outputWallet)
	if err != nil {
		return iotago.EmptyTransactionID, ierrors.Wrapf(err, "failed to create splitted outputs")
	}

	genesisAccount := e.accManager.FaucetRequestsAccount()
	if genesisAccount == nil {
		return iotago.EmptyTransactionID, ierrors.New("failed to split output, genesis account is nil")
	}

	issuanceData, err := e.CreateTransaction(ctx,
		WithInputs(splitOutput),
		WithOutputs(outputs),
		WithInputWallet(inputWallet),
		WithOutputWallet(outputWallet),
	)
	if err != nil {
		return iotago.EmptyTransactionID, err
	}

	_, tx, err := e.PrepareAndPostBlockWithTxBuildData(ctx, e.connector.GetClient(), issuanceData.TransactionBuilder, genesisAccount)
	if err != nil {
		return iotago.EmptyTransactionID, err
	}

	txID, err := tx.ID()
	if err != nil {
		return iotago.EmptyTransactionID, err
	}

	e.LogDebugf("Splitting output %s finished with tx: %s", splitOutput.OutputID.ToHex(), txID.ToHex())

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

	e.LogDebugf("Splitting %d outputs from wallet no %d", len(inputWallet.UnspentOutputs()), inputWallet.ID)
	txIDs := make([]iotago.TransactionID, 0)
	wg := sync.WaitGroup{}
	// split all outputs stored in the input wallet
	for id := range inputWallet.UnspentOutputs() {
		wg.Add(1)

		go func(id models.TempOutputID) {
			defer wg.Done()

			input := inputWallet.UnspentOutput(id)
			txID, err := e.splitOutput(ctx, input, inputWallet, outputWallet)
			if err != nil {
				e.LogErrorf("Failed to split output %s: %s", input.OutputID.ToHex(), err)

				return
			}
			txIDs = append(txIDs, txID)
		}(id)
	}
	wg.Wait()
	e.LogDebug("All blocks with splitting transactions were posted")

	e.outputManager.AwaitTransactionsAcceptance(ctx, txIDs...)

	return txIDs, nil
}

func (e *EvilWallet) createSplitOutputs(input *models.OutputData, receiveWallet *Wallet) ([]*OutputOption, error) {
	totalAmount := input.OutputStruct.BaseTokenAmount()
	splitNumber := e.faucetSplitNumber
	minDeposit := e.minOutputStorageDeposit

	// make sure the amount of output covers the min deposit
	amountPerOutput, err := safemath.SafeDiv(totalAmount, iotago.BaseToken(splitNumber))
	if err != nil {
		e.LogErrorf("Failed to calculate amount per output, total amount %d, splitted amount %d: %s", totalAmount, 128, err)
	}

	if amountPerOutput < minDeposit {
		outputsNum, err := safemath.SafeDiv(totalAmount, minDeposit)
		if err != nil {
			e.LogErrorf("Failed to calculate split number, total amount %d, splitted amount %d: %s", totalAmount, minDeposit, err)

			return nil, ierrors.Wrapf(err, "failed to calculate split number")
		}

		splitNumber = int(outputsNum)
	}

	balances := utils.SplitBalanceEqually(splitNumber, input.OutputStruct.BaseTokenAmount())
	manaBalances := utils.SplitBalanceEqually(splitNumber, input.OutputStruct.StoredMana())
	outputs := make([]*OutputOption, splitNumber)
	for i, bal := range balances {
		outputs[i] = &OutputOption{amount: bal, mana: manaBalances[i], address: receiveWallet.Address(), outputType: iotago.OutputBasic}
	}

	return outputs, nil
}
