package accountwallet

import (
	"context"
	"crypto/ed25519"
	"os"
	"sync"

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
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/iotaledger/iota.go/v4/wallet"
)

func Run(ctx context.Context, logger log.Logger, opts ...options.Option[AccountWallets]) (*AccountWallets, error) {
	w, err := NewAccountWallets(ctx, logger, opts...)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to create wallet")
	}

	// load wallet
	err = w.fromAccountStateFile(ctx)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to load wallet from file")
	}

	return w, nil
}

func SaveState(w *AccountWallets) error {
	return w.toAccountStateFile()
}

type GenesisAccountParams struct {
	FaucetPrivateKey string
	FaucetAccountID  string
}

type AccountWallet struct {
	seed [32]byte

	alias       string
	accountData *models.AccountData
	outputs     []*models.OutputData

	latestUsedIndex atomic.Uint32

	GenesisAccount wallet.Account // can be used by any wallet as an external block issuer
	Client         *models.WebClient
	API            iotago.API
	log.Logger
}

type AccountWallets struct {
	wallets      map[string]*AccountWallet
	walletsMutex sync.RWMutex

	log.Logger

	GenesisAccount wallet.Account

	optsClientBindAddress    string
	optsFaucetURL            string
	optsAccountStatesFile    string
	optsGenesisAccountParams *GenesisAccountParams

	client             *models.WebClient
	API                iotago.API
	RequestTokenAmount iotago.BaseToken
	RequestManaAmount  iotago.Mana
}

func NewAccountWallets(_ context.Context, logger log.Logger, opts ...options.Option[AccountWallets]) (*AccountWallets, error) {
	accountWalletLogger := logger.NewChildLogger("AccountWallet")

	var initErr error

	return options.Apply(&AccountWallets{
		wallets: make(map[string]*AccountWallet),
		Logger:  accountWalletLogger,
	}, opts, func(w *AccountWallets) {
		w.Client, initErr = models.NewWebClient(w.optsClientBindAddress, w.optsFaucetURL)
		if initErr != nil {
			accountWalletLogger.LogErrorf("failed to create web client: %s", initErr.Error())

			return
		}
		w.API = w.Client.LatestAPI()

		genesisAccountData := &models.AccountData{
			Account:  lo.PanicOnErr(wallet.AccountFromParams(w.optsGenesisAccountParams.FaucetAccountID, w.optsGenesisAccountParams.FaucetPrivateKey)),
			Status:   models.AccountReady,
			OutputID: iotago.EmptyOutputID,
			Index:    0,
		}
		w.GenesisAccount = genesisAccountData.Account
		genesisWallet := w.NewAccountWallet(GenesisAccountAlias, genesisAccountData)

		// determine the faucet request amounts
		_, output, err := genesisWallet.RequestFaucetFunds(context.Background(), tpkg.RandEd25519Address())
		if err != nil {
			initErr = err
			accountWalletLogger.LogErrorf("failed to request faucet funds: %s, faucet not initiated", err.Error())

			return
		}
		w.RequestTokenAmount = output.BaseTokenAmount()
		w.RequestManaAmount = output.StoredMana()

		accountWalletLogger.LogDebugf("faucet initiated with %d tokens and %d mana", w.RequestTokenAmount, w.RequestManaAmount)

	}), initErr

}

func (a *AccountWallets) NewAccountWallet(alias string, accountData *models.AccountData) *AccountWallet {
	a.walletsMutex.Lock()
	defer a.walletsMutex.Unlock()

	return a.newAccountWallet(alias, accountData)
}

func (a *AccountWallets) newAccountWallet(alias string, accountData *models.AccountData) *AccountWallet {
	accountWallet := &AccountWallet{
		alias:       alias,
		outputs:     make([]*models.OutputData, 0),
		seed:        tpkg.RandEd25519Seed(),
		accountData: accountData,
		client:      a.client,
		API:         a.API,
		Logger:      a.Logger,
	}

	a.wallets[alias] = accountWallet

	return accountWallet
}

func (a *AccountWallets) GetOrCreateWallet(alias string) *AccountWallet {
	a.walletsMutex.Lock()
	defer a.walletsMutex.Unlock()

	wallet, exists := a.wallets[alias]
	if !exists {
		return a.newAccountWallet(alias, nil)
	}

	return wallet
}

func (a *AccountWallet) AccountData() *models.AccountData {
	return a.accountData
}

// toAccountStateFile write account states to file.
func (a *AccountWallets) toAccountStateFile() error {
	a.walletsMutex.Lock()
	defer a.walletsMutex.Unlock()

	walletStates := make([]*models.WalletState, 0)

	for _, wallet := range a.wallets {
		walletState := &models.WalletState{
			Alias:         wallet.alias,
			Seed:          base58.Encode(wallet.seed[:]),
			LastUsedIndex: wallet.latestUsedIndex.Load(),
		}
		if wallet.accountData != nil {
			walletState.AccountState = models.AccountStateFromAccountData(wallet.accountData)
		}
		for _, output := range wallet.outputs {
			walletState.OutputStates = append(walletState.OutputStates, models.OutputStateFromOutputData(output))
		}

		walletStates = append(walletStates, walletState)
	}
	stateBytes, err := a.Client.LatestAPI().Encode(&StateData{
		Wallets: walletStates,
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

func (a *AccountWallets) fromAccountStateFile(ctx context.Context) error {
	a.walletsMutex.Lock()
	defer a.walletsMutex.Unlock()

	walletStateBytes, err := os.ReadFile(a.optsAccountStatesFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return ierrors.Wrap(err, "failed to read file")
		}

		return nil
	}

	var data StateData
	_, err = a.Client.LatestAPI().Decode(walletStateBytes, &data)
	if err != nil {
		return ierrors.Wrap(err, "failed to decode from file")
	}
	for _, walletState := range data.Wallets {
		a.LogDebugf("Loading walletState: %+v\n", walletState)
		wallet := &AccountWallet{
			alias:          walletState.Alias,
			GenesisAccount: a.GenesisAccount,
			client:         a.client,
			API:            a.API,
			Logger:         a.Logger,
		}
		// copy seed
		decodedSeeds, err := base58.Decode(walletState.Seed)
		if err != nil {
			return ierrors.Wrap(err, "failed to decode seed")
		}
		copy(wallet.seed[:], decodedSeeds)
		// copy account
		if len(walletState.AccountState) != 0 {
			wallet.accountData = walletState.AccountState[0].ToAccountData()
		}
		for _, outputState := range walletState.OutputStates {
			wallet.outputs = append(wallet.outputs, a.OutputStateToOutputData(ctx, outputState))
		}
		a.wallets[walletState.Alias] = wallet
	}

	return nil
}

func (a *AccountWallets) OutputStateToOutputData(ctx context.Context, o *models.OutputState) *models.OutputData {
	output := a.client.GetOutput(ctx, o.OutputID)
	return &models.OutputData{
		OutputID:     o.OutputID,
		Address:      output.UnlockConditionSet().Address().Address,
		AddressIndex: o.Index,
		PrivateKey:   o.PrivateKey,
		OutputStruct: output,
	}
}

//nolint:all,unused
func (a *AccountWallets) registerAccount(alias string, accountID iotago.AccountID, outputID iotago.OutputID, index uint32, privateKey ed25519.PrivateKey) iotago.AccountID {
	a.walletsMutex.Lock()
	defer a.walletsMutex.Unlock()

	account := wallet.NewEd25519Account(accountID, privateKey)
	accountData := &models.AccountData{
		Account:  account,
		Status:   models.AccountPending,
		OutputID: outputID,
		Index:    index,
	}
	wallet, exists := a.wallets[alias]
	if exists {
		wallet.accountData = accountData
		a.LogDebugf("overwriting account %s with alias %s\noutputID: %s addr: %s\n", accountID.String(), alias, outputID.ToHex(), account.Address().String())
		return accountID
	}

	a.wallets[alias] = a.newAccountWallet(alias, accountData)

	return accountID

}

func (a *AccountWallets) deleteAccount(alias string) {
	a.walletsMutex.Lock()
	defer a.walletsMutex.Unlock()

	wallet, exists := a.wallets[alias]
	if !exists {
		return
	}
	wallet.accountData = nil

	a.LogDebugf("deleting account with alias %s", alias)
}

func (a *AccountWallets) registerOutput(alias string, output *models.OutputData) {
	a.walletsMutex.Lock()
	defer a.walletsMutex.Unlock()

	if _, exists := a.wallets[alias]; !exists {
		a.wallets[alias] = a.newAccountWallet(alias, nil)
	}
	a.wallets[alias].outputs = append(a.wallets[alias].outputs, output)
}

// checkOutputStatus checks the status of an output by requesting all possible endpoints.
func (a *AccountWallet) checkOutputStatus(ctx context.Context, blkID iotago.BlockID, txID iotago.TransactionID, creationOutputID iotago.OutputID, accountAddress *iotago.AccountAddress, checkIndexer ...bool) error {
	// request by blockID if provided, otherwise use txID
	slot := blkID.Slot()
	if blkID == iotago.EmptyBlockID {
		blkMetadata, err := a.Client.GetBlockStateFromTransaction(ctx, txID)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get block state from transaction %s", txID.ToHex())
		}
		blkID = blkMetadata.BlockID
		slot = blkMetadata.BlockID.Slot()
	}

	if err := utils.AwaitBlockAndPayloadAcceptance(ctx, a.Logger, a.Client, blkID); err != nil {
		return ierrors.Wrapf(err, "failed to await block issuance for block %s", blkID.ToHex())
	}
	a.LogInfof("Block and Transaction accepted: blockID %s", blkID.ToHex())

	// wait for the account to be committed
	if accountAddress != nil {
		a.LogInfof("Checking for commitment of account, blk ID: %s, txID: %s and creation output: %s\nBech addr: %s", blkID.ToHex(), txID.ToHex(), creationOutputID.ToHex(), accountAddress.Bech32(a.Client.CommittedAPI().ProtocolParameters().Bech32HRP()))
	} else {
		a.LogInfof("Checking for commitment of output, blk ID: %s, txID: %s and creation output: %s", blkID.ToHex(), txID.ToHex(), creationOutputID.ToHex())
	}
	err := utils.AwaitCommitment(ctx, a.Logger, a.Client, slot)
	if err != nil {
		a.LogErrorf("Failed to await commitment for slot %d: %s", slot, err)

		return err
	}

	// Check the indexer
	if len(checkIndexer) > 0 && checkIndexer[0] {
		outputID, account, _, err := a.Client.GetAccountFromIndexer(ctx, accountAddress)
		if err != nil {
			a.LogDebugf("Failed to get account from indexer, even after slot %d is already committed", slot)
			return ierrors.Wrapf(err, "failed to get account from indexer, even after slot %d is already committed", slot)
		}

		a.LogDebugf("Indexer returned: outputID %s, account %s, slot %d", outputID.String(), account.AccountID.ToAddress().Bech32(a.Client.CommittedAPI().ProtocolParameters().Bech32HRP()), slot)
	}

	// check if the creation output exists
	outputFromNode, err := a.Client.Client().OutputByID(ctx, creationOutputID)
	if err != nil {
		a.LogDebugf("Failed to get output from node, even after slot %d is already committed", slot)
		return ierrors.Wrapf(err, "failed to get output from node, even after slot %d is already committed", slot)
	}
	a.LogDebugf("Node returned: outputID %s, output %s", creationOutputID.ToHex(), outputFromNode.Type())

	if accountAddress != nil {
		a.LogInfof("Account present in commitment for slot %d\nBech addr: %s", slot, accountAddress.Bech32(a.Client.CommittedAPI().ProtocolParameters().Bech32HRP()))
	} else {
		a.LogInfof("Output present in commitment for slot %d", slot)
	}

	return nil
}

func (a *AccountWallet) updateAccountStatus(status models.AccountStatus) (updated bool) {
	if a.accountData == nil {
		return false
	}

	if a.accountData.Status == status {
		return false
	}

	a.accountData.Status = status

	return true
}

func (a *AccountWallet) GetReadyAccount(ctx context.Context) (*models.AccountData, error) {

	if a.accountData.Status == models.AccountReady {
		return a.accountData, nil
	}

	ready := a.awaitAccountReadiness(ctx, a.accountData)
	if !ready {
		return nil, ierrors.Errorf("account with accountID %s is not ready", a.accountData.Account.ID())
	}

	return a.accountData, nil
}

func (a *AccountWallets) GetAccount(alias string) (*models.AccountData, error) {
	a.walletsMutex.RLock()
	defer a.walletsMutex.RUnlock()

	wallet, exists := a.wallets[alias]
	if !exists {
		return nil, ierrors.Errorf("wallet with alias %s does not exist", alias)
	}
	if wallet.accountData == nil {
		return nil, ierrors.Errorf("account with alias %s does not exist", alias)
	}

	return wallet.accountData, nil
}

func (a *AccountWallets) GetDelegations(alias string) ([]*models.OutputData, error) {
	a.walletsMutex.RLock()
	defer a.walletsMutex.RUnlock()

	wallet, exists := a.wallets[alias]
	if !exists {
		return nil, ierrors.Errorf("wallet with alias %s does not exist", alias)
	}
	if len(wallet.outputs) == 0 {
		return nil, ierrors.Errorf("delegations with alias %s do not exist", alias)
	}
	delegations := make([]*models.OutputData, 0)
	for _, output := range wallet.outputs {
		if output.OutputStruct.Type() == iotago.OutputDelegation {
			delegations = append(delegations, output)
		}
	}

	return delegations, nil
}

func (a *AccountWallet) awaitAccountReadiness(ctx context.Context, accData *models.AccountData) bool {
	creationSlot := accData.OutputID.CreationSlot()
	a.LogInfof("Waiting for account to be committed within slot %d...", creationSlot)
	err := utils.AwaitCommitment(ctx, a.Logger, a.Client, creationSlot)
	if err != nil {
		a.LogErrorf("failed to get commitment details while waiting: %s", err)

		return false
	}

	return a.updateAccountStatus(models.AccountReady)
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
