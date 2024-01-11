package accountwallet

import (
	"context"
	"crypto/ed25519"
	"os"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/mr-tron/base58"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-crypto-demo/pkg/bip32path"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/iotaledger/iota.go/v4/wallet"
)

func Run(logger log.Logger, opts ...options.Option[AccountWallet]) (*AccountWallet, error) {
	w, err := NewAccountWallet(logger, opts...)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to create wallet")
	}

	// load wallet
	err = w.fromAccountStateFile()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to load wallet from file")
	}

	return w, nil
}

func SaveState(w *AccountWallet) error {
	return w.toAccountStateFile()
}

type GenesisAccountParams struct {
	FaucetPrivateKey string
	FaucetAccountID  string
}

type AccountWallet struct {
	log.Logger
	GenesisAccount wallet.Account
	seed           [32]byte

	accountsAliases     map[string]*models.AccountData
	accountAliasesMutex sync.RWMutex

	latestUsedIndex atomic.Uint32

	client             *models.WebClient
	API                iotago.API
	RequestTokenAmount iotago.BaseToken
	RequestManaAmount  iotago.Mana

	optsClientBindAddress    string
	optsFaucetURL            string
	optsAccountStatesFile    string
	optsGenesisAccountParams *GenesisAccountParams
}

func NewAccountWallet(logger log.Logger, opts ...options.Option[AccountWallet]) (*AccountWallet, error) {
	accountWalletLogger := logger.NewChildLogger("AccountWallet")

	var initErr error

	return options.Apply(&AccountWallet{
		Logger:          accountWalletLogger,
		accountsAliases: make(map[string]*models.AccountData),
		seed:            tpkg.RandEd25519Seed(),
	}, opts, func(w *AccountWallet) {
		w.client, initErr = models.NewWebClient(w.optsClientBindAddress, w.optsFaucetURL)
		if initErr != nil {
			accountWalletLogger.LogErrorf("failed to create web client: %s", initErr.Error())

			return
		}
		w.API = w.client.LatestAPI()

		w.GenesisAccount = lo.PanicOnErr(wallet.AccountFromParams(w.optsGenesisAccountParams.FaucetAccountID, w.optsGenesisAccountParams.FaucetPrivateKey))

		// determine the faucet request amounts
		_, output, err := w.RequestFaucetFunds(context.Background(), tpkg.RandEd25519Address())
		if err != nil {
			initErr = err
			accountWalletLogger.LogErrorf("failed to request faucet funds: %s, faucet not initiated", err.Error())

			return
		}
		w.RequestTokenAmount = output.BaseTokenAmount()
		w.RequestManaAmount = output.StoredMana()

		// add genesis account
		accountWalletLogger.LogDebugf("faucet initiated with %d tokens and %d mana", w.RequestTokenAmount, w.RequestManaAmount)
		w.accountsAliases[GenesisAccountAlias] = &models.AccountData{
			Alias:    GenesisAccountAlias,
			Status:   models.AccountReady,
			OutputID: iotago.EmptyOutputID,
			Index:    0,
			Account:  w.GenesisAccount,
		}
	}), initErr
}

// toAccountStateFile write account states to file.
func (a *AccountWallet) toAccountStateFile() error {
	a.accountAliasesMutex.Lock()
	defer a.accountAliasesMutex.Unlock()

	accounts := make([]*models.AccountState, 0)

	for _, acc := range a.accountsAliases {
		accounts = append(accounts, models.AccountStateFromAccountData(acc))
	}

	stateBytes, err := a.client.LatestAPI().Encode(&StateData{
		Seed:          base58.Encode(a.seed[:]),
		LastUsedIndex: a.latestUsedIndex.Load(),
		AccountsData:  accounts,
	})
	if err != nil {
		return ierrors.Wrap(err, "failed to encode state")
	}

	//nolint:gosec // users should be able to read the file
	if err = os.WriteFile(a.optsAccountStatesFile, stateBytes, 0o644); err != nil {
		return ierrors.Wrap(err, "failed to write account states to file")
	}

	return nil
}

func (a *AccountWallet) fromAccountStateFile() error {
	a.accountAliasesMutex.Lock()
	defer a.accountAliasesMutex.Unlock()

	walletStateBytes, err := os.ReadFile(a.optsAccountStatesFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return ierrors.Wrap(err, "failed to read file")
		}

		return nil
	}

	var data StateData
	_, err = a.client.LatestAPI().Decode(walletStateBytes, &data)
	if err != nil {
		return ierrors.Wrap(err, "failed to decode from file")
	}

	// copy seeds
	decodedSeeds, err := base58.Decode(data.Seed)
	if err != nil {
		return ierrors.Wrap(err, "failed to decode seed")
	}
	copy(a.seed[:], decodedSeeds)

	// set latest used index
	a.latestUsedIndex.Store(data.LastUsedIndex)

	// account data
	for _, acc := range data.AccountsData {
		a.accountsAliases[acc.Alias] = acc.ToAccountData()
		if acc.Alias == GenesisAccountAlias {
			a.accountsAliases[acc.Alias].Status = models.AccountReady
		}
	}

	return nil
}

//nolint:all,unused
func (a *AccountWallet) registerAccount(alias string, accountID iotago.AccountID, outputID iotago.OutputID, index uint32, privateKey ed25519.PrivateKey) iotago.AccountID {
	a.accountAliasesMutex.Lock()
	defer a.accountAliasesMutex.Unlock()

	account := wallet.NewEd25519Account(accountID, privateKey)
	a.accountsAliases[alias] = &models.AccountData{
		Alias:    alias,
		Account:  account,
		Status:   models.AccountPending,
		OutputID: outputID,
		Index:    index,
	}
	a.LogDebugf("registering account %s with alias %s\noutputID: %s addr: %s\n", accountID.String(), alias, outputID.ToHex(), account.Address().String())

	return accountID
}

// checkAccountStatus checks the status of the account by requesting all possible endpoints.
func (a *AccountWallet) checkAccountStatus(ctx context.Context, blkID iotago.BlockID, txID iotago.TransactionID, creationOutputID iotago.OutputID, accountAddress *iotago.AccountAddress, checkIndexer ...bool) error {
	// request by blockID if provided, otherwise use txID
	slot := blkID.Slot()
	if blkID == iotago.EmptyBlockID {
		blkMetadata, err := a.client.GetBlockStateFromTransaction(ctx, txID)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get block state from transaction %s", txID.ToHex())
		}
		blkID = blkMetadata.BlockID
		slot = blkMetadata.BlockID.Slot()
	}

	if err := utils.AwaitBlockAndPayloadAcceptance(ctx, a.Logger, a.client, blkID); err != nil {
		return ierrors.Wrapf(err, "failed to await block issuance for block %s", blkID.ToHex())
	}
	a.LogInfof("Block and Transaction accepted: blockID %s", blkID.ToHex())

	// wait for the account to be committed
	if accountAddress != nil {
		a.LogInfof("Checking for commitment of account, blk ID: %s, txID: %s and creation output: %s\nBech addr: %s", blkID.ToHex(), txID.ToHex(), creationOutputID.ToHex(), accountAddress.Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))
	} else {
		a.LogInfof("Checking for commitment of output, blk ID: %s, txID: %s and creation output: %s", blkID.ToHex(), txID.ToHex(), creationOutputID.ToHex())
	}
	err := utils.AwaitCommitment(ctx, a.Logger, a.client, slot)
	if err != nil {
		a.LogErrorf("Failed to await commitment for slot %d: %s", slot, err)

		return err
	}

	// Check the indexer
	if len(checkIndexer) > 0 && checkIndexer[0] {
		outputID, account, _, err := a.client.GetAccountFromIndexer(ctx, accountAddress)
		if err != nil {
			a.LogDebugf("Failed to get account from indexer, even after slot %d is already committed", slot)
			return ierrors.Wrapf(err, "failed to get account from indexer, even after slot %d is already committed", slot)
		}

		a.LogDebugf("Indexer returned: outputID %s, account %s, slot %d", outputID.String(), account.AccountID.ToAddress().Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()), slot)
	}

	// check if the creation output exists
	outputFromNode, err := a.client.Client().OutputByID(ctx, creationOutputID)
	if err != nil {
		a.LogDebugf("Failed to get output from node, even after slot %d is already committed", slot)
		return ierrors.Wrapf(err, "failed to get output from node, even after slot %d is already committed", slot)
	}
	a.LogDebugf("Node returned: outputID %s, output %s", creationOutputID.ToHex(), outputFromNode.Type())

	if accountAddress != nil {
		a.LogInfof("Account present in commitment for slot %d\nBech addr: %s", slot, accountAddress.Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))
	} else {
		a.LogInfof("Output present in commitment for slot %d", slot)
	}

	return nil
}

func (a *AccountWallet) updateAccountStatus(alias string, status models.AccountStatus) (updated bool) {
	a.accountAliasesMutex.Lock()
	defer a.accountAliasesMutex.Unlock()

	accData, exists := a.accountsAliases[alias]
	if !exists {
		return false
	}

	if accData.Status == status {
		return false
	}

	accData.Status = status
	a.accountsAliases[alias] = accData

	return true
}

func (a *AccountWallet) GetReadyAccount(ctx context.Context, alias string) (*models.AccountData, error) {
	accData, err := a.GetAccount(alias)
	if err != nil {
		return nil, ierrors.Wrapf(err, "account with alias %s does not exist", alias)
	}

	if accData.Status == models.AccountReady {
		return accData, nil
	}

	ready := a.awaitAccountReadiness(ctx, accData)
	if !ready {
		return nil, ierrors.Errorf("account with alias %s is not ready", alias)
	}

	return accData, nil
}

func (a *AccountWallet) GetAccount(alias string) (*models.AccountData, error) {
	a.accountAliasesMutex.RLock()
	defer a.accountAliasesMutex.RUnlock()

	accData, exists := a.accountsAliases[alias]
	if !exists {
		return nil, ierrors.Errorf("account with alias %s does not exist", alias)
	}

	return accData, nil
}

func (a *AccountWallet) awaitAccountReadiness(ctx context.Context, accData *models.AccountData) bool {
	creationSlot := accData.OutputID.CreationSlot()
	a.LogInfof("Waiting for account %s to be committed within slot %d...", accData.Alias, creationSlot)
	err := utils.AwaitCommitment(ctx, a.Logger, a.client, creationSlot)
	if err != nil {
		a.LogErrorf("failed to get commitment details while waiting %s: %s", accData.Alias, err)

		return false
	}

	return a.updateAccountStatus(accData.Alias, models.AccountReady)
}

func (a *AccountWallet) destroyAccount(ctx context.Context, alias string) error {
	accData, err := a.GetAccount(alias)
	if err != nil {
		return err
	}
	// get output from node
	// From TIP42: Indexers and node plugins shall map the account address of the output derived with Account ID to the regular address -> output mapping table, so that given an Account Address, its most recent unspent account output can be retrieved.
	// TODO: use correct outputID
	accountOutput := a.client.GetOutput(ctx, accData.OutputID)
	switch accountOutput.Type() {
	case iotago.OutputBasic:
		a.LogInfof("Cannot destroy implicit account %s", alias)

		return nil
	}

	keyManager, err := wallet.NewKeyManager(a.seed[:], BIP32PathForIndex(accData.Index))
	if err != nil {
		return err
	}
	{
		// first, transition the account so block issuer feature expires if it is not already.
		issuingTime := time.Now()
		issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
		apiForSlot := a.client.APIForSlot(issuingSlot)
		// get the latest block issuance data from the node
		congestionResp, issuerResp, version, err := a.RequestBlockIssuanceData(ctx, a.client, a.GenesisAccount)
		if err != nil {
			return ierrors.Wrap(err, "failed to request block built data for the faucet account")
		}
		commitmentID := lo.Return1(issuerResp.LatestCommitment.ID())
		commitmentSlot := commitmentID.Slot()
		pastBoundedSlot := commitmentSlot + apiForSlot.ProtocolParameters().MaxCommittableAge()
		// transition it to expire if it is not already as soon as possible
		if accountOutput.FeatureSet().BlockIssuer().ExpirySlot > pastBoundedSlot {
			// start building the transaction
			txBuilder := builder.NewTransactionBuilder(apiForSlot)
			// add the account output as input
			txBuilder.AddInput(&builder.TxInput{
				UnlockTarget: accountOutput.UnlockConditionSet().Address().Address,
				InputID:      accData.OutputID,
				Input:        accountOutput,
			})
			// create an account output with updated expiry slot set to commitment slot + MaxCommittableAge (pastBoundedSlot)
			// nolint:forcetypeassert // we know that this is an account output
			accountBuilder := builder.NewAccountOutputBuilderFromPrevious(accountOutput.(*iotago.AccountOutput))
			accountBuilder.BlockIssuer(accountOutput.FeatureSet().BlockIssuer().BlockIssuerKeys, pastBoundedSlot)
			expiredAccountOutput := accountBuilder.MustBuild()
			// add the expired account output as output
			txBuilder.AddOutput(expiredAccountOutput)
			// add the commitment input
			txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: commitmentID})
			// add a block issuance credit input
			txBuilder.AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: accData.Account.ID()})
			// set the creation slot to the issuance slot
			txBuilder.SetCreationSlot(issuingSlot)
			// set the transaction capabilities to be able to do anything
			txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
			// build the transaction
			signedTx, err := txBuilder.Build(keyManager.AddressSigner())
			if err != nil {
				return ierrors.Wrap(err, "failed to build transaction")
			}

			// issue the transaction in a block
			blockID, err := a.PostWithBlock(ctx, a.client, signedTx, a.GenesisAccount, congestionResp, issuerResp, version)
			if err != nil {
				return ierrors.Wrapf(err, "failed to post block with ID %s", blockID)
			}
			a.LogInfof("Posted transaction: transition account to expire in slot %d\nBech addr: %s", pastBoundedSlot, accData.Account.Address().Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))

			// check the status of the transaction
			expiredAccountOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
			err = a.checkAccountStatus(ctx, blockID, lo.PanicOnErr(signedTx.Transaction.ID()), expiredAccountOutputID, accData.Account.Address())
			if err != nil {
				return ierrors.Wrap(err, "failure checking for commitment of account transition")
			}

			// update the account output details in the wallet
			a.registerAccount(alias, accData.Account.ID(), expiredAccountOutputID, accData.Index, accData.Account.PrivateKey())
		}
		// wait until the expiry time has passed
		if time.Now().Before(apiForSlot.TimeProvider().SlotEndTime(pastBoundedSlot)) {
			a.LogInfof("Waiting for slot %d when account expires", pastBoundedSlot)
			time.Sleep(time.Until(apiForSlot.TimeProvider().SlotEndTime(pastBoundedSlot)))
		}
	}
	{
		// next, issue a transaction to destroy the account output
		issuingTime := time.Now()
		issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
		apiForSlot := a.client.APIForSlot(issuingSlot)
		// get the latest block issuance data from the node
		congestionResp, issuerResp, version, err := a.RequestBlockIssuanceData(ctx, a.client, a.GenesisAccount)
		if err != nil {
			return ierrors.Wrap(err, "failed to request block built data for the faucet account")
		}
		commitmentID := lo.Return1(issuerResp.LatestCommitment.ID())
		// start building the transaction
		txBuilder := builder.NewTransactionBuilder(apiForSlot)
		// add the expired account output on the input side
		accData, err := a.GetAccount(alias)
		if err != nil {
			return err
		}
		expiredAccountOutput := a.client.GetOutput(ctx, accData.OutputID)
		txBuilder.AddInput(&builder.TxInput{
			UnlockTarget: expiredAccountOutput.UnlockConditionSet().Address().Address,
			InputID:      accData.OutputID,
			Input:        expiredAccountOutput,
		})
		// add a basic output to output side
		addr, _, _ := a.getAddress(iotago.AddressEd25519)
		basicOutput := builder.NewBasicOutputBuilder(addr, expiredAccountOutput.BaseTokenAmount()).MustBuild()
		txBuilder.AddOutput(basicOutput)
		// set the creation slot to the issuance slot
		txBuilder.SetCreationSlot(issuingSlot)
		// add the commitment input
		txBuilder.AddCommitmentInput(&iotago.CommitmentInput{CommitmentID: commitmentID})
		// add a block issuance credit input
		txBuilder.AddBlockIssuanceCreditInput(&iotago.BlockIssuanceCreditInput{AccountID: accData.Account.ID()})
		// set the transaction capabilities to be able to do anything
		txBuilder.WithTransactionCapabilities(iotago.TransactionCapabilitiesBitMaskWithCapabilities(iotago.WithTransactionCanDoAnything()))
		// build the transaction
		signedTx, err := txBuilder.Build(keyManager.AddressSigner())
		if err != nil {
			return ierrors.Wrap(err, "failed to build transaction")
		}
		// issue the transaction in a block
		blockID, err := a.PostWithBlock(ctx, a.client, signedTx, a.GenesisAccount, congestionResp, issuerResp, version)
		if err != nil {
			return ierrors.Wrapf(err, "failed to post block with ID %s", blockID)
		}
		a.LogInfof("Posted transaction: destroy account\nBech addr: %s", accData.Account.Address().Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()))

		// check the status of the transaction
		basicOutputID := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), 0)
		err = a.checkAccountStatus(ctx, blockID, lo.PanicOnErr(signedTx.Transaction.ID()), basicOutputID, nil)
		if err != nil {
			return ierrors.Wrap(err, "failure checking for commitment of account transition")
		}

		// check that the basic output is retrievable
		// TODO: move this to checkIndexer within the checkAccountStatus function
		// check for the basic output being committed, indicating the account output has been consumed (destroyed)
		if output := a.client.GetOutput(ctx, basicOutputID); output == nil {
			return ierrors.Wrap(err, "failed to get basic output from node after commitment")
		}

		// remove account from wallet
		delete(a.accountsAliases, alias)

		a.LogInfof("Account %s has been destroyed", alias)
	}

	return nil
}

func BIP32PathForIndex(index uint32) string {
	path := lo.PanicOnErr(bip32path.ParsePath(wallet.DefaultIOTAPath))
	if len(path) != 5 {
		panic("invalid path length")
	}

	// Set the index
	path[4] = index | (1 << 31)

	return path.String()
}
