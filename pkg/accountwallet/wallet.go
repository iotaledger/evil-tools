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
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/iotaledger/iota.go/v4/wallet"
)

var log = utils.NewLogger("AccountWallet")

func Run(config *Configuration) (*AccountWallet, error) {
	var opts []options.Option[AccountWallet]
	if config.BindAddress != "" {
		opts = append(opts, WithClientURL(config.BindAddress))
	}
	if config.FaucetBindAddress != "" {
		opts = append(opts, WithFaucetURL(config.FaucetBindAddress))
	}
	if config.AccountStatesFile != "" {
		opts = append(opts, WithAccountStatesFile(config.AccountStatesFile))
	}

	opts = append(opts, WithFaucetAccountParams(&faucetParams{
		genesisSeed:      config.GenesisSeed,
		faucetPrivateKey: config.BlockIssuerPrivateKey,
		faucetAccountID:  config.AccountID,
	}))

	w, err := NewAccountWallet(opts...)
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

type AccountWallet struct {
	faucet *faucet
	seed   [32]byte

	accountsAliases     map[string]*models.AccountData
	accountAliasesMutex sync.RWMutex

	latestUsedIndex atomic.Uint64

	client *models.WebClient
	API    iotago.API

	optsClientBindAddress string
	optsFaucetURL         string
	optsAccountStatesFile string
	optsFaucetParams      *faucetParams
}

func NewAccountWallet(opts ...options.Option[AccountWallet]) (*AccountWallet, error) {
	var initErr error
	return options.Apply(&AccountWallet{
		accountsAliases: make(map[string]*models.AccountData),
		seed:            tpkg.RandEd25519Seed(),
	}, opts, func(w *AccountWallet) {
		w.client, initErr = models.NewWebClient(w.optsClientBindAddress, w.optsFaucetURL)
		if initErr != nil {
			log.Errorf("failed to create web client: %s", initErr.Error())

			return
		}
		w.API = w.client.LatestAPI()

		w.faucet = newFaucet(w.client, w.optsFaucetParams)

		//
		//_, output, err := w.RequestFaucetFunds(context.Background(), tpkg.RandEd25519Address())
		//if err != nil {
		//	initErr = err
		//	log.Errorf("failed to request faucet funds: %s, faucet not initiated", err.Error())
		//
		//	return
		//}
		//w.faucet.RequestTokenAmount = output.BaseTokenAmount()
		//w.faucet.RequestManaAmount = output.StoredMana()

		w.faucet.RequestTokenAmount = 1000000000000
		w.faucet.RequestManaAmount = 100000000

		log.Debugf("faucet initiated with %d tokens and %d mana", w.faucet.RequestTokenAmount, w.faucet.RequestManaAmount)
		w.accountsAliases[GenesisAccountAlias] = &models.AccountData{
			Alias:    GenesisAccountAlias,
			Status:   models.AccountReady,
			OutputID: iotago.EmptyOutputID,
			Index:    0,
			Account:  w.faucet.account,
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
func (a *AccountWallet) registerAccount(alias string, accountID iotago.AccountID, outputID iotago.OutputID, index uint64, privateKey ed25519.PrivateKey) iotago.AccountID {
	a.accountAliasesMutex.Lock()
	defer a.accountAliasesMutex.Unlock()

	account := wallet.NewEd25519Account(accountID, privateKey)
	log.Debugf("registering account %s with alias %s\noutputID: %s addr: %s\n", accountID.String(), alias, outputID.String(), account.Address().String())
	a.accountsAliases[alias] = &models.AccountData{
		Alias:    alias,
		Account:  account,
		Status:   models.AccountPending,
		OutputID: outputID,
		Index:    index,
	}

	return accountID
}

// checkAccountStatus checks the status of the account by requesting all possible endpoints.
// TODO it is not throwing an error if any API request fails, beside the blockMetadata, only logs it, after all is fixed it should fail on any response error.
func (a *AccountWallet) checkAccountStatus(ctx context.Context, blkID iotago.BlockID, txID iotago.TransactionID, creationOutputID iotago.OutputID, accountAddress *iotago.AccountAddress, accountID iotago.AccountID) error {
	// request by blockID if provided, otherwise use txID
	var slot iotago.SlotIndex
	if blkID != iotago.EmptyBlockID {
		if err := utils.AwaitBlockAndPayloadAcceptance(ctx, a.client, blkID); err != nil {
			return ierrors.Wrapf(err, "failed to await block issuance for block %s", blkID.ToHex())
		}
		slot = blkID.Slot()
	} else {
		slot = txID.Slot() + 6 // just in case tx was created much before the block
	}

	log.Infof("Created account with addr: %s, accID: %s blk ID: %s, txID: %s and creation output: %s awaiting the commitment.", accountAddress.Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()), accountID, blkID.ToHex(), txID.ToHex(), creationOutputID.ToHex())

	// wait for the account to be committed
	err := utils.AwaitCommitment(ctx, a.client, slot)
	if err != nil {
		log.Errorf("Failed to await commitment for slot %d: %s", slot, err)

		return err
	}
	log.Infof("Slot %d is committed", slot)

	if blkID == iotago.EmptyBlockID {
		resp, err := a.client.GetBlockStateFromTransaction(ctx, txID)
		if err != nil {
			log.Debugf("RequestFaucetFunds faucet tx state: %s, block state: %s, tx failure: %d, block failure: %d", resp.TransactionMetadata.TransactionState, resp.BlockState, resp.TransactionMetadata, resp.BlockFailureReason)

			return ierrors.Wrap(err, "failed to get block state from transaction")
		}
	}

	// Check the indexer
	outputID, account, _, err := a.client.GetAccountFromIndexer(ctx, accountAddress)
	if err != nil {
		log.Debugf("Failed to get account from indexer, even after slot %d is already committed", slot)

		//return err
	} else {
		log.Debugf("Indexer returned: outputID %s, account %s, slot %d", outputID.String(), account.AccountID, slot)
	}

	// check if the creation output exists
	outputFromNode, err := a.client.Client().OutputByID(ctx, creationOutputID)
	if err != nil {
		log.Debugf("Failed to get output from node, even after slot %d is already committed", slot)
	} else {
		log.Debugf("Node returned: outputID %s, output %s", creationOutputID, outputFromNode.Type())
	}

	log.Infof("Account created, Bech addr: %s, slot: %d", accountAddress.Bech32(a.client.CommittedAPI().ProtocolParameters().Bech32HRP()), slot)

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
	log.Infof("Waiting for account %s to be committed within slot %d...", accData.Alias, creationSlot)
	err := utils.AwaitCommitment(ctx, a.client, creationSlot)
	if err != nil {
		log.Errorf("failed to get commitment details while waiting %s: %s", accData.Alias, err)

		return false
	}

	return a.updateAccountStatus(accData.Alias, models.AccountReady)
}

func (a *AccountWallet) destroyAccount(ctx context.Context, alias string) error {
	accData, err := a.GetAccount(alias)
	if err != nil {
		return err
	}

	keyManager, err := wallet.NewKeyManager(a.seed[:], accData.Index)
	if err != nil {
		return err
	}

	issuingTime := time.Now()
	issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
	apiForSlot := a.client.APIForSlot(issuingSlot)

	// get output from node
	// From TIP42: Indexers and node plugins shall map the account address of the output derived with Account ID to the regular address -> output mapping table, so that given an Account Address, its most recent unspent account output can be retrieved.
	// TODO: use correct outputID
	accountOutput := a.client.GetOutput(ctx, accData.OutputID)

	txBuilder := builder.NewTransactionBuilder(apiForSlot)
	txBuilder.AddInput(&builder.TxInput{
		UnlockTarget: a.accountsAliases[alias].Account.Address(),
		InputID:      accData.OutputID,
		Input:        accountOutput,
	})

	// send all tokens to faucet
	//nolint:all,forcetypassert
	txBuilder.AddOutput(&iotago.BasicOutput{
		Amount: accountOutput.BaseTokenAmount(),
		UnlockConditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: a.faucet.genesisKeyManager.Address(iotago.AddressEd25519).(*iotago.Ed25519Address)},
		},
	})

	tx, err := txBuilder.Build(keyManager.AddressSigner())
	if err != nil {
		return ierrors.Wrapf(err, "failed to build transaction for account alias destruction %s", alias)
	}

	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(ctx, a.client, a.faucet.account)
	if err != nil {
		return ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}

	blockID, err := a.PostWithBlock(ctx, a.client, tx, a.faucet.account, congestionResp, issuerResp, version)
	if err != nil {
		return ierrors.Wrapf(err, "failed to post block with ID %s", blockID)
	}

	// remove account from wallet
	delete(a.accountsAliases, alias)

	log.Infof("Account %s has been destroyed", alias)

	return nil
}
