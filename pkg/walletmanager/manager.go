package walletmanager

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
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/iotaledger/iota.go/v4/wallet"
)

const (
	GenesisAccountAlias = "genesis-account"
	FaucetRequestsAlias = "faucet-requests"
)

type GenesisAccountParams struct {
	GenesisPrivateKey string
	GenesisAccountID  string
	FaucetPrivateKey  string
	FaucetAccountID   string
}

func NewGenesisAccountParams(params *models.ParametersTool) *GenesisAccountParams {
	g := &GenesisAccountParams{
		GenesisPrivateKey: params.BlockIssuerPrivateKey,
		GenesisAccountID:  params.AccountID,
		FaucetPrivateKey:  params.BlockIssuerPrivateKey,
		FaucetAccountID:   params.AccountID,
	}
	// use default genesis accounts if the faucet specific were not provided
	if params.FaucetRequestsAccountID == "" || params.FaucetRequestsBlockIssuerPrivateKey == "" {
		g.FaucetAccountID = g.GenesisAccountID
		g.FaucetPrivateKey = g.GenesisPrivateKey
	}

	return g
}

func RunManager(logger log.Logger, opts ...options.Option[Manager]) (*Manager, error) {
	m, err := newManager(logger, opts...)
	if err != nil {
		return nil, err
	}

	loadedFromFile, err := m.LoadStateFromFile()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to load wallet from file, please delete the file and try again")
	}

	if !loadedFromFile {
		m.LogInfo("No wallet state file found, creating new wallet")
		err = m.setupRequestsAmounts()
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

type Manager struct {
	accounts    map[string]*models.AccountData // not serializable with serix
	wallets     map[string]*Wallet
	delegations map[string][]*Delegation

	RequestTokenAmount iotago.BaseToken
	RequestManaAmount  iotago.Mana

	optsClientBindAddress    string
	optsFaucetURL            string
	optsAccountStatesFile    string
	optsGenesisAccountParams *GenesisAccountParams
	optsSilence              bool

	Client *models.WebClient
	API    iotago.API
	sync.RWMutex
	log.Logger
}

func newManager(logger log.Logger, opts ...options.Option[Manager]) (*Manager, error) {
	managerLogger := logger.NewChildLogger("accounts")
	var innerErr error

	return options.Apply(&Manager{
		accounts:    make(map[string]*models.AccountData),
		wallets:     make(map[string]*Wallet),
		delegations: make(map[string][]*Delegation),
		Logger:      managerLogger,
	}, opts, func(m *Manager) {
		err := m.setupClient()
		if err != nil {
			innerErr = err

			return
		}

		m.setupGenesisAccount()
	}), innerErr
}

func (m *Manager) setupClient() error {
	var err error
	m.Client, err = models.NewWebClient(m.optsClientBindAddress, m.optsFaucetURL)
	if err != nil {
		m.LogErrorf("failed to create web client: %s", err.Error())

		return ierrors.Wrap(err, "failed to create web client")
	}

	m.API = m.Client.LatestAPI()

	return nil
}

func (m *Manager) setupGenesisAccount() {
	genesisAccountData := &models.AccountData{
		Account:  lo.PanicOnErr(wallet.AccountFromParams(m.optsGenesisAccountParams.GenesisAccountID, m.optsGenesisAccountParams.GenesisPrivateKey)),
		Status:   models.AccountReady,
		OutputID: iotago.EmptyOutputID,
		Index:    0,
	}
	m.AddAccount(GenesisAccountAlias, genesisAccountData)

	faucetRequestsAccountData := &models.AccountData{
		Account:  lo.PanicOnErr(wallet.AccountFromParams(m.optsGenesisAccountParams.FaucetAccountID, m.optsGenesisAccountParams.FaucetPrivateKey)),
		Status:   models.AccountReady,
		OutputID: iotago.EmptyOutputID,
		Index:    0,
	}
	m.AddAccount(FaucetRequestsAlias, faucetRequestsAccountData)
}

func (m *Manager) setupRequestsAmounts() error {
	if m.optsSilence {
		// skip the request
		return nil
	}

	_, output, err := m.RequestFaucetFunds(context.Background(), m.Client, tpkg.RandEd25519Address())
	if err != nil {
		m.LogErrorf("failed to request faucet funds: %s, faucet not initiated", err.Error())

		return err
	}
	m.RequestTokenAmount = output.BaseTokenAmount()
	m.RequestManaAmount = output.StoredMana()

	m.LogDebugf("faucet initiated with %d tokens and %d mana", m.RequestTokenAmount, m.RequestManaAmount)

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

func (m *Manager) AddWallet(alias string, wallet *Wallet) {
	m.Lock()
	defer m.Unlock()

	m.wallets[alias] = wallet
}

func (m *Manager) GetWallet(alias string) (*Wallet, error) {
	m.RLock()
	defer m.RUnlock()

	w, exists := m.wallets[alias]
	if !exists {
		return nil, ierrors.Errorf("wallet with alias %s does not exist", alias)
	}

	return w, nil
}

func (m *Manager) GenesisAccount() wallet.Account {
	return lo.Return1(m.GetAccount(GenesisAccountAlias)).Account
}

func (m *Manager) FaucetRequestsAccount() wallet.Account {
	return lo.Return1(m.GetAccount(FaucetRequestsAlias)).Account
}

func (m *Manager) GetDelegations(alias string) ([]*Delegation, error) {
	m.RLock()
	defer m.RUnlock()

	delegations, exist := m.delegations[alias]
	if !exist {
		return nil, ierrors.Errorf("delegations with alias %s does not exist", alias)
	}

	return delegations, nil
}

func (m *Manager) removeDelegations(alias string) {
	m.Lock()
	defer m.Unlock()

	delete(m.delegations, alias)
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
	return accountID

}

func (m *Manager) deleteAccount(alias string) {
	m.Lock()
	defer m.Unlock()

	delete(m.accounts, alias)

	m.LogDebugf("deleting account with alias %s", alias)
}

func (m *Manager) registerDelegationOutput(alias string, output *models.OutputData) {
	m.Lock()
	defer m.Unlock()

	if _, exists := m.wallets[alias]; !exists {
		panic("wallet should already exist!")
	}

	_, ok := m.delegations[alias]
	if !ok {
		m.delegations[alias] = make([]*Delegation, 0)
	}

	m.delegations[alias] = append(m.delegations[alias], &Delegation{
		Alias:                  alias,
		OutputID:               output.OutputID,
		AddressIndex:           output.AddressIndex,
		Amount:                 output.OutputStruct.BaseTokenAmount(),
		DelegatedToBechAddress: output.Address.Bech32(m.Client.CommittedAPI().ProtocolParameters().Bech32HRP()),
	})
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

func WithSilence() options.Option[Manager] {
	return func(a *Manager) {
		a.optsSilence = true
	}
}
