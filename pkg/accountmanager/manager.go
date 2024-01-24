package accountmanager

import (
	"context"
	"crypto/ed25519"
	"sync"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/wallet"
)

const GenesisAccountAlias = "genesis-account"

type GenesisAccountParams struct {
	FaucetPrivateKey string
	FaucetAccountID  string
}

func Run(ctx context.Context, logger log.Logger, opts ...options.Option[Manager]) (*Manager, error) {
	m, err := NewManager(logger, opts...)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to create wallet")
	}

	loadedFromFile, err := m.LoadStateFromFile()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to load wallet from file")
	}
	if !loadedFromFile {
		m.LogInfo("No wallet state file found, creating new wallet")
		err = m.SetupFromParams()
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

type Manager struct {
	accounts           map[string]*models.AccountData // not serializable with serix
	wallets            map[string]*AccountWallet      `serix:"wallets,lenPrefix=uint8"`
	RequestTokenAmount iotago.BaseToken               `serix:"RequestTokenAmount"`
	RequestManaAmount  iotago.Mana                    `serix:"RequestManaAmount"`

	optsClientBindAddress    string
	optsFaucetURL            string
	optsAccountStatesFile    string
	optsGenesisAccountParams *GenesisAccountParams

	Client *models.WebClient
	API    iotago.API
	sync.RWMutex
	log.Logger
}

func NewManager(logger log.Logger, opts ...options.Option[Manager]) (*Manager, error) {
	managerLogger := logger.NewChildLogger("AccountWallet")

	return options.Apply(&Manager{
		accounts: make(map[string]*models.AccountData),
		wallets:  make(map[string]*AccountWallet),
		Logger:   managerLogger,
	}, opts), nil
}

func (m *Manager) SetupFromParams() error {
	var err error
	m.Client, err = models.NewWebClient(m.optsClientBindAddress, m.optsFaucetURL)
	if err != nil {
		m.LogErrorf("failed to create web client: %s", err.Error())

		return nil
	}
	m.API = m.Client.LatestAPI()

	genesisAccountData := &models.AccountData{
		Account:  lo.PanicOnErr(wallet.AccountFromParams(m.optsGenesisAccountParams.FaucetAccountID, m.optsGenesisAccountParams.FaucetPrivateKey)),
		Status:   models.AccountReady,
		OutputID: iotago.EmptyOutputID,
		Index:    0,
	}
	m.AddAccount(GenesisAccountAlias, genesisAccountData)

	// TODO determine the faucet request amounts
	//_, output, err := m.RequestFaucetFunds(context.Background(), m.Client, tpkg.RandEd25519Address())
	//if err != nil {
	//	m.LogErrorf("failed to request faucet funds: %s, faucet not initiated", err.Error())
	//
	//	return err
	//}
	//m.RequestTokenAmount = output.BaseTokenAmount()
	//m.RequestManaAmount = output.StoredMana()
	//
	//accountWalletLogger.LogDebugf("faucet initiated with %d tokens and %d mana", w.RequestTokenAmount, w.RequestManaAmount)

	return nil
}

func (m *Manager) AddAccount(alias string, data *models.AccountData) {
	m.Lock()
	defer m.Unlock()

	m.accounts[alias] = data
}

func (m *Manager) GetAccount(alias string) (*models.AccountData, error) {
	m.RLock()
	defer m.RUnlock()

	accData, exists := m.accounts[alias]
	if !exists {
		return nil, ierrors.Errorf("account with alias %s does not exist", alias)
	}

	return accData, nil
}

func (m *Manager) GetReadyAccount(ctx context.Context, clt models.Client, alias string) (*models.AccountData, error) {
	accountData, err := m.GetAccount(alias)
	if err != nil {
		return nil, err
	}
	if accountData.Status == models.AccountReady {
		return accountData, nil
	}

	ready := m.awaitAccountReadiness(ctx, clt, alias, accountData)
	if !ready {
		return nil, ierrors.Errorf("account with accountID %s is not ready", accountData.Account.ID())
	}

	return accountData, nil
}

func (m *Manager) AddWallet(alias string, wallet *AccountWallet) {
	m.Lock()
	defer m.Unlock()

	m.wallets[alias] = wallet
}

func (m *Manager) GetWallet(alias string) (*AccountWallet, error) {
	m.RLock()
	defer m.RUnlock()

	w, exists := m.wallets[alias]
	if !exists {
		return nil, ierrors.Errorf("wallet with alias %s does not exist", alias)
	}

	return w, nil
}

func (m *Manager) GenesisAccount() wallet.Account {
	return lo.Return1(m.accounts[GenesisAccountAlias]).Account
}

func (m *Manager) GetDelegations(alias string) ([]*models.OutputData, error) {
	m.RLock()
	defer m.RUnlock()

	w, exists := m.wallets[alias]
	if !exists {
		return nil, ierrors.Errorf("wallet with alias %s does not exist", alias)
	}
	if len(w.delegationOutputs) == 0 {
		return nil, ierrors.Errorf("delegations with alias %s do not exist", alias)
	}
	delegations := make([]*models.OutputData, 0)
	for _, output := range w.delegationOutputs {
		if output.OutputStruct.Type() == iotago.OutputDelegation {
			delegations = append(delegations, output)
		}
	}

	return delegations, nil
}

//nolint:all,unused
func (m *Manager) registerAccount(alias string, accountID iotago.AccountID, outputID iotago.OutputID, index uint32, privateKey ed25519.PrivateKey) iotago.AccountID {
	m.Lock()
	defer m.Unlock()

	account := wallet.NewEd25519Account(accountID, privateKey)
	accountData := &models.AccountData{
		Account:  account,
		Status:   models.AccountPending,
		OutputID: outputID,
		Index:    index,
	}
	accountData, exists := m.accounts[alias]
	if exists {
		m.accounts[alias] = accountData
		m.LogDebugf("overwriting account %s with alias %s\noutputID: %s addr: %s\n", accountID.String(), alias, outputID.ToHex(), account.Address().String())
		return accountID
	}

	m.wallets[alias] = m.newAccountWallet(alias)

	return accountID

}

func (m *Manager) deleteAccount(alias string) {
	m.Lock()
	defer m.Unlock()

	delete(m.accounts, alias)

	m.LogDebugf("deleting account with alias %s", alias)
}

func (m *Manager) registerOutput(alias string, output *models.OutputData) {
	m.Lock()
	defer m.Unlock()

	if _, exists := m.wallets[alias]; !exists {
		m.wallets[alias] = m.newAccountWallet(alias)
	}
	m.wallets[alias].delegationOutputs = append(m.wallets[alias].delegationOutputs, output)
}

func (m *Manager) awaitAccountReadiness(ctx context.Context, clt models.Client, alias string, accData *models.AccountData) bool {
	creationSlot := accData.OutputID.CreationSlot()
	m.LogInfof("Waiting for account to be committed within slot %d...", creationSlot)
	err := utils.AwaitCommitment(ctx, m.Logger, clt, creationSlot)
	if err != nil {
		m.LogErrorf("failed to get commitment details while waiting: %s", err)

		return false
	}

	return m.updateAccountStatus(alias, models.AccountReady)
}

func (m *Manager) updateAccountStatus(alias string, status models.AccountStatus) (updated bool) {
	m.Lock()
	defer m.Unlock()

	a, exists := m.accounts[alias]
	if !exists {
		return false
	}

	if a.Status == status {
		return false
	}

	a.Status = status

	return true
}

// WithClientURL sets the client bind address.
func WithClientURL(url string) options.Option[Manager] {
	return func(a *Manager) {
		a.optsClientBindAddress = url
	}
}

func WithFaucetURL(url string) options.Option[Manager] {
	return func(a *Manager) {
		a.optsFaucetURL = url
	}
}

func WithAccountStatesFile(fileName string) options.Option[Manager] {
	return func(a *Manager) {
		a.optsAccountStatesFile = fileName
	}
}

func WithFaucetAccountParams(params *GenesisAccountParams) options.Option[Manager] {
	return func(a *Manager) {
		a.optsGenesisAccountParams = params
	}
}
