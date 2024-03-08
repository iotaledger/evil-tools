package walletmanager

import (
	"context"
	"time"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

func (m *Manager) allot(ctx context.Context, params *AllotAccountParams) error {
	w, err := m.GetWallet(params.Alias)
	if err != nil {
		return ierrors.Wrapf(err, "could not get wallet and account for alias %s", params.Alias)
	}
	account, err := m.GetAccount(params.Alias)
	if err != nil {
		return ierrors.Wrapf(err, "could not get account for alias %s", params.Alias)
	}

	inputs, err := m.RequestManaAndFundsFromTheFaucet(ctx, w, params.Amount, iotago.BaseToken(0))
	if err != nil {
		return ierrors.Wrapf(err, "failed to request mana and funds from the faucet for alias %s", params.Alias)
	}
	congestionResp, issuanceResp, version, err := m.RequestBlockIssuanceData(ctx, m.Client, account.Account)
	if err != nil {
		return ierrors.Wrapf(err, "failed to request block issuance data for alias %s", params.Alias)
	}

	signedTx, err := m.createAllotTransaction(inputs, w, account.Account.ID())
	if err != nil {
		return ierrors.Wrapf(err, "failed to create transaction with allotment to alias %s", params.Alias)
	}

	blockID, err := m.PostWithBlock(ctx, m.Client, signedTx, m.GenesisAccount(), congestionResp, issuanceResp, version)
	if err != nil {
		return ierrors.Wrapf(err, "failed to post transaction with allotment to alias %s", params.Alias)
	}

	m.LogInfof("Posted transaction with blockID %s: allotment to alias %s", blockID.ToHex(), params.Alias)

	return nil
}

func (m *Manager) createAllotTransaction(inputs []*models.OutputData, w *Wallet, accountID iotago.AccountID) (*iotago.SignedTransaction, error) {
	currentTime := time.Now()
	currentSlot := m.API.TimeProvider().SlotFromTime(currentTime)
	apiForSlot := m.Client.APIForSlot(currentSlot)

	// transaction signer
	addrSigner, err := w.GetAddrSignerForIndexes(inputs...)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get address signer")
	}

	txBuilder := builder.NewTransactionBuilder(apiForSlot, addrSigner)
	for _, output := range inputs {
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: output.Address,
			InputID:      output.OutputID,
			Input:        output.OutputStruct,
		})
	}
	totalBalance := utils.SumOutputsBalance(inputs)
	outputAddr, _, _ := w.getAddress(iotago.AddressEd25519)
	output := builder.NewBasicOutputBuilder(outputAddr, totalBalance).MustBuild()
	txBuilder.
		AddOutput(output).
		SetCreationSlot(currentSlot).
		//WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
		AllotAllMana(txBuilder.CreationSlot(), accountID, 0)

	signedTx, err := txBuilder.Build()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to build tx")
	}

	return signedTx, nil
}
