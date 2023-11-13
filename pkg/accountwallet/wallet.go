package accountwallet

import (
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
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
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

	wallet, err := NewAccountWallet(opts...)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to create wallet")
	}

	// load wallet
	err = wallet.fromAccountStateFile()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to load wallet from file")
	}

	return wallet, nil
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

	optsClientBindAddress string
	optsFaucetURL         string
	optsAccountStatesFile string
	optsFaucetParams      *faucetParams
	optsRequestTimeout    time.Duration
	optsRequestTicker     time.Duration
}

func NewAccountWallet(opts ...options.Option[AccountWallet]) (*AccountWallet, error) {
	var initErr error
	return options.Apply(&AccountWallet{
		accountsAliases:    make(map[string]*models.AccountData),
		seed:               tpkg.RandEd25519Seed(),
		optsRequestTimeout: time.Second * 120,
		optsRequestTicker:  time.Second * 5,
	}, opts, func(w *AccountWallet) {
		w.client, initErr = models.NewWebClient(w.optsClientBindAddress, w.optsFaucetURL)
		if initErr != nil {
			log.Errorf("failed to create web client: %s", initErr.Error())

			return
		}

		out, err := w.RequestFaucetFunds(tpkg.RandAddress())
		if err != nil {
			log.Errorf("failed to request faucet funds: %s, faucet not initiated", err.Error())

			return
		}
		var f *faucet
		f, initErr = newFaucet(w.client, w.optsFaucetParams, out.Balance, out.OutputStruct.StoredMana())
		if initErr != nil {
			return
		}

		w.faucet = f
		w.accountsAliases[FaucetAccountAlias] = &models.AccountData{
			Alias:    FaucetAccountAlias,
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
		if acc.Alias == FaucetAccountAlias {
			a.accountsAliases[acc.Alias].Status = models.AccountReady
		}
	}

	return nil
}

//nolint:all,unused
func (a *AccountWallet) registerAccount(alias string, outputID iotago.OutputID, index uint64, privKey ed25519.PrivateKey) iotago.AccountID {
	a.accountAliasesMutex.Lock()
	defer a.accountAliasesMutex.Unlock()

	accountID := iotago.AccountIDFromOutputID(outputID)
	account := mock.NewEd25519Account(accountID, privKey)

	a.accountsAliases[alias] = &models.AccountData{
		Alias:    alias,
		Account:  account,
		Status:   models.AccountPending,
		OutputID: outputID,
		Index:    index,
	}

	return accountID
}

func (a *AccountWallet) updateAccountStatus(alias string, status models.AccountStatus) (*models.AccountData, bool) {
	a.accountAliasesMutex.Lock()
	defer a.accountAliasesMutex.Unlock()

	accData, exists := a.accountsAliases[alias]
	if !exists {
		return nil, false
	}

	if accData.Status == status {
		return accData, false
	}

	accData.Status = status
	a.accountsAliases[alias] = accData

	return accData, true
}

func (a *AccountWallet) GetReadyAccount(alias string) (*models.AccountData, error) {
	a.accountAliasesMutex.Lock()
	defer a.accountAliasesMutex.Unlock()

	accData, exists := a.accountsAliases[alias]
	if !exists {
		return nil, ierrors.Errorf("account with alias %s does not exist", alias)
	}

	// check if account is ready (to be included in a commitment)
	ready := a.isAccountReady(accData)
	if !ready {
		return nil, ierrors.Errorf("account with alias %s is not ready", alias)
	}

	accData, _ = a.updateAccountStatus(alias, models.AccountReady)

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

func (a *AccountWallet) isAccountReady(accData *models.AccountData) bool {
	if accData.Status == models.AccountReady {
		return true
	}

	creationSlot := accData.OutputID.CreationSlot()

	// wait for the account to be committed
	log.Infof("Waiting for account %s to be committed within slot %d...", accData.Alias, creationSlot)
	err := a.retry(func() (bool, error) {
		resp, err := a.client.GetBlockIssuance()
		if err != nil {
			return false, err
		}

		if resp.Commitment.Slot >= creationSlot {
			log.Infof("Slot %d committed, account %s is ready to use", creationSlot, accData.Alias)
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		log.Errorf("failed to get commitment details while waiting %s: %s", accData.Alias, err)
		return false
	}

	return true
}

func (a *AccountWallet) getFunds(addressType iotago.AddressType) (*models.Output, ed25519.PrivateKey, error) {
	receiverAddr, privKey, usedIndex := a.getAddress(addressType)

	createdOutput, err := a.RequestFaucetFunds(receiverAddr)
	if err != nil {
		return nil, nil, ierrors.Wrap(err, "failed to request funds from Faucet")
	}

	createdOutput.AddressIndex = usedIndex
	createdOutput.PrivKey = privKey

	return createdOutput, privKey, nil
}

func (a *AccountWallet) destroyAccount(alias string) error {
	accData, err := a.GetAccount(alias)
	if err != nil {
		return err
	}
	hdWallet := mock.NewKeyManager(a.seed[:], accData.Index)

	issuingTime := time.Now()
	issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
	apiForSlot := a.client.APIForSlot(issuingSlot)

	// get output from node
	// From TIP42: Indexers and node plugins shall map the account address of the output derived with Account ID to the regular address -> output mapping table, so that given an Account Address, its most recent unspent account output can be retrieved.
	// TODO: use correct outputID
	accountOutput := a.client.GetOutput(accData.OutputID)

	txBuilder := builder.NewTransactionBuilder(apiForSlot)
	txBuilder.AddInput(&builder.TxInput{
		UnlockTarget: a.accountsAliases[alias].Account.ID().ToAddress(),
		InputID:      accData.OutputID,
		Input:        accountOutput,
	})

	// send all tokens to faucet
	//nolint:all,forcetypassert
	txBuilder.AddOutput(&iotago.BasicOutput{
		Amount: accountOutput.BaseTokenAmount(),
		Conditions: iotago.BasicOutputUnlockConditions{
			&iotago.AddressUnlockCondition{Address: a.faucet.genesisHdWallet.Address(iotago.AddressEd25519).(*iotago.Ed25519Address)},
		},
	})

	tx, err := txBuilder.Build(hdWallet.AddressSigner())
	if err != nil {
		return ierrors.Wrapf(err, "failed to build transaction for account alias destruction %s", alias)
	}

	congestionResp, issuerResp, version, err := a.RequestBlockBuiltData(a.client.Client(), a.faucet.account.ID())
	if err != nil {
		return ierrors.Wrap(err, "failed to request block built data for the faucet account")
	}

	blockID, err := a.PostWithBlock(a.client, tx, a.faucet.account, congestionResp, issuerResp, version)
	if err != nil {
		return ierrors.Wrapf(err, "failed to post block with ID %s", blockID)
	}

	// remove account from wallet
	delete(a.accountsAliases, alias)

	log.Infof("Account %s has been destroyed", alias)

	return nil
}

func (a *AccountWallet) retry(requestFunc func() (bool, error)) error {
	timeout := time.NewTimer(a.optsRequestTimeout)
	interval := time.NewTicker(a.optsRequestTicker)
	defer timeutil.CleanupTimer(timeout)
	defer timeutil.CleanupTicker(interval)

	for {
		done, err := requestFunc()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-interval.C:
			continue
		case <-timeout.C:
			return ierrors.New("timeout while trying to request")
		}
	}
}
