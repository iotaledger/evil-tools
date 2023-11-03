package evilwallet

import (
	"time"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockhandler"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

const (
	MinOutputStorageDeposit = iotago.BaseToken(500)
	// MaxBigWalletsCreatedAtOnce is maximum of evil wallets that can be created at once for non-infinite spam.
	MaxBigWalletsCreatedAtOnce = 10
	// BigFaucetWalletDeposit indicates the minimum outputs left number that triggers funds requesting in the background.
	BigFaucetWalletDeposit = 4
	// CheckFundsLeftInterval is the interval to check funds left in the background for requesting funds triggering.
	CheckFundsLeftInterval = time.Second * 5
	// BigFaucetWalletsAtOnce number of faucet wallets requested at once in the background.
	BigFaucetWalletsAtOnce = 2
)

var (
	defaultClientsURLs = []string{"http://localhost:8050", "http://localhost:8060"}
	defaultFaucetURL   = "http://localhost:8088"
)

// region EvilWallet ///////////////////////////////////////////////////////////////////////////////////////////////////////

// EvilWallet provides a user-friendly way to do complicated double spend scenarios.
type EvilWallet struct {
	wallets       *Wallets
	accWallet     *accountwallet.AccountWallet
	connector     models.Connector
	outputManager *OutputManager
	aliasManager  *AliasManager

	minOutputStorageDeposit iotago.BaseToken

	optsClientURLs []string
	optsFaucetURL  string
	log            *logger.Logger
}

// NewEvilWallet creates an EvilWallet instance.
func NewEvilWallet(opts ...options.Option[EvilWallet]) *EvilWallet {
	return options.Apply(&EvilWallet{
		wallets:                 NewWallets(),
		aliasManager:            NewAliasManager(),
		minOutputStorageDeposit: MinOutputStorageDeposit,
		optsClientURLs:          defaultClientsURLs,
		optsFaucetURL:           defaultFaucetURL,
		log:                     utils.NewLogger("EvilWallet"),
	}, opts, func(w *EvilWallet) {
		connector := models.NewWebClients(w.optsClientURLs, w.optsFaucetURL)
		w.connector = connector
		w.outputManager = NewOutputManager(connector, w.wallets, w.log)

		// Get output storage deposit at start
		minOutputStorageDeposit, err := w.connector.GetClient().CommittedAPI().StorageScoreStructure().MinDeposit(tpkg.RandBasicOutput(iotago.AddressEd25519))
		if err == nil {
			w.minOutputStorageDeposit = minOutputStorageDeposit
		}
	})
}

// NewWallet creates a new wallet of the given wallet type.
func (e *EvilWallet) NewWallet(wType ...WalletType) *Wallet {
	walletType := Other
	if len(wType) != 0 {
		walletType = wType[0]
	}

	return e.wallets.NewWallet(walletType)
}

// GetClients returns the given number of clients.
func (e *EvilWallet) GetClients(num int) []models.Client {
	return e.connector.GetClients(num)
}

// Connector give access to the EvilWallet connector.
func (e *EvilWallet) Connector() models.Connector {
	return e.connector
}

func (e *EvilWallet) UnspentOutputsLeft(walletType WalletType) int {
	return e.wallets.UnspentOutputsLeft(walletType)
}

func (e *EvilWallet) NumOfClient() int {
	clients := e.connector.Clients()
	return len(clients)
}

func (e *EvilWallet) AddClient(clientURL string) {
	e.connector.AddClient(clientURL)
}

func (e *EvilWallet) RemoveClient(clientURL string) {
	e.connector.RemoveClient(clientURL)
}

func (e *EvilWallet) GetAccount(alias string) (blockhandler.Account, error) {
	account, err := e.accWallet.GetAccount(alias)
	if err != nil {
		return nil, err
	}

	return account.Account, nil
}

func (e *EvilWallet) CreateBlock(clt models.Client, payload iotago.Payload, congestionResp *apimodels.CongestionResponse, issuer blockhandler.Account, strongParents ...iotago.BlockID) (*iotago.ProtocolBlock, error) {
	var congestionSlot iotago.SlotIndex
	version := clt.CommittedAPI().Version()
	if congestionResp != nil {
		congestionSlot = congestionResp.Slot
		version = clt.APIForSlot(congestionSlot).Version()
	}

	issuerResp, err := clt.GetBlockIssuance(congestionSlot)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get block issuance data")
	}

	block, err := e.accWallet.CreateBlock(payload, issuer, congestionResp, issuerResp, version, strongParents...)
	if err != nil {
		return nil, err
	}

	return block, nil
}

func (e *EvilWallet) PrepareAndPostBlock(clt models.Client, payload iotago.Payload, congestionResp *apimodels.CongestionResponse, issuer blockhandler.Account) (iotago.BlockID, error) {
	var congestionSlot iotago.SlotIndex
	version := clt.CommittedAPI().Version()
	if congestionResp != nil {
		congestionSlot = congestionResp.Slot
		version = clt.APIForSlot(congestionSlot).Version()
	}

	issuerResp, err := clt.GetBlockIssuance(congestionSlot)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrap(err, "failed to get block issuance data")
	}

	blockID, err := e.accWallet.PostWithBlock(clt, payload, issuer, congestionResp, issuerResp, version)
	if err != nil {
		return iotago.EmptyBlockID, err
	}

	return blockID, nil
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region EvilWallet functionality ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// ClearAliases remove only provided aliases from AliasManager.
func (e *EvilWallet) ClearAliases(aliases ScenarioAlias) {
	e.aliasManager.ClearAliases(aliases)
}

// ClearAllAliases remove all registered alias names.
func (e *EvilWallet) ClearAllAliases() {
	e.aliasManager.ClearAllAliases()
}

func (e *EvilWallet) PrepareCustomConflicts(conflictsMaps []ConflictSlice) (conflictBatch [][]*models.PayloadIssuanceData, err error) {
	for _, conflictMap := range conflictsMaps {
		var txsData []*models.PayloadIssuanceData
		for _, conflictOptions := range conflictMap {
			txData, err2 := e.CreateTransaction(conflictOptions...)
			if err2 != nil {
				return nil, err2
			}
			txsData = append(txsData, txData)
		}
		conflictBatch = append(conflictBatch, txsData)
	}

	return conflictBatch, nil
}

// CreateTransaction creates a transaction based on provided options. If no input wallet is provided, the next non-empty faucet wallet is used.
// Inputs of the transaction are determined in three ways:
// 1 - inputs are provided directly without associated alias, 2- provided only an alias, but inputs are stored in an alias manager,
// 3 - provided alias, but there are no inputs assigned in Alias manager, so aliases will be assigned to next ready inputs from input wallet.
func (e *EvilWallet) CreateTransaction(options ...Option) (*models.PayloadIssuanceData, error) {
	buildOptions, err := NewOptions(options...)
	if err != nil {
		return nil, err
	}
	// wallet used only for outputs in the middle of the batch, that will never be reused outside custom conflict batch creation.
	tempWallet := e.NewWallet()

	err = e.updateInputWallet(buildOptions)
	if err != nil {
		return nil, err
	}

	inputs, err := e.prepareInputs(buildOptions)
	if err != nil {
		return nil, err
	}

	outputs, addrAliasMap, tempAddresses, err := e.prepareOutputs(buildOptions, tempWallet)
	if err != nil {
		return nil, err
	}

	alias, remainder, remainderAddr, hasRemainder := e.prepareRemainderOutput(buildOptions, outputs)
	if hasRemainder {
		outputs = append(outputs, remainder)
		if alias != "" && addrAliasMap != nil {
			addrAliasMap[remainderAddr.String()] = alias
		}
	}

	var congestionResp *apimodels.CongestionResponse
	// request congestion endpoint if allotment strategy configured
	if buildOptions.allotmentStrategy == models.AllotmentStrategyMinCost {
		congestionResp, err = e.connector.GetClient().GetCongestion(buildOptions.issuerAccountID)
		if err != nil {
			return nil, err
		}
	}

	signedTx, err := e.makeTransaction(inputs, outputs, buildOptions.inputWallet, congestionResp, buildOptions.issuerAccountID)
	if err != nil {
		return nil, err
	}

	txData := &models.PayloadIssuanceData{
		Payload:            signedTx,
		CongestionResponse: congestionResp,
	}

	e.addOutputsToOutputManager(signedTx, buildOptions.outputWallet, tempWallet, tempAddresses)
	e.registerOutputAliases(signedTx, addrAliasMap)

	//e.log.Debugf("\n %s", printTransaction(signedTx))

	return txData, nil
}

// addOutputsToOutputManager adds output to the OutputManager if.
func (e *EvilWallet) addOutputsToOutputManager(signedTx *iotago.SignedTransaction, outWallet, tmpWallet *Wallet, tempAddresses map[string]types.Empty) {
	for idx, o := range signedTx.Transaction.Outputs {
		if o.UnlockConditionSet().Address() == nil {
			continue
		}

		// register UnlockConditionAddress only (skip account outputs)
		addr := o.UnlockConditionSet().Address().Address
		out := &models.Output{
			OutputID:     iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), uint16(idx)),
			Address:      addr,
			Balance:      o.BaseTokenAmount(),
			OutputStruct: o,
		}

		if _, ok := tempAddresses[addr.String()]; ok {
			e.outputManager.AddOutput(tmpWallet, out)
		} else {
			out.AddressIndex = outWallet.AddrIndexMap(addr.String())
			e.outputManager.AddOutput(outWallet, out)
		}
	}
}

// updateInputWallet if input wallet is not specified, or aliases were provided without inputs (batch inputs) use Fresh faucet wallet.
func (e *EvilWallet) updateInputWallet(buildOptions *Options) error {
	for alias := range buildOptions.aliasInputs {
		// inputs provided for aliases (middle inputs in a batch)
		_, ok := e.aliasManager.GetInput(alias)
		if ok {
			// leave nil, wallet will be selected based on OutputIDWalletMap
			buildOptions.inputWallet = nil

			return nil
		}

		break
	}
	wallet, err := e.useFreshIfInputWalletNotProvided(buildOptions)

	if err != nil {
		return err
	}

	buildOptions.inputWallet = wallet

	return nil
}

func (e *EvilWallet) registerOutputAliases(signedTx *iotago.SignedTransaction, addrAliasMap map[string]string) {
	if len(addrAliasMap) == 0 {
		return
	}

	for idx := range signedTx.Transaction.Outputs {
		id := iotago.OutputIDFromTransactionIDAndIndex(lo.PanicOnErr(signedTx.Transaction.ID()), uint16(idx))
		out := e.outputManager.GetOutput(id)
		if out == nil {
			continue
		}

		// register output alias
		e.aliasManager.AddOutputAlias(out, addrAliasMap[out.Address.String()])

		// register output as unspent output(input)
		e.aliasManager.AddInputAlias(out, addrAliasMap[out.Address.String()])
	}
}

func (e *EvilWallet) prepareInputs(buildOptions *Options) (inputs []*models.Output, err error) {
	if buildOptions.areInputsProvidedWithoutAliases() {
		inputs = append(inputs, buildOptions.inputs...)

		return
	}
	// append inputs with alias
	aliasInputs, err := e.matchInputsWithAliases(buildOptions)
	if err != nil {
		return nil, err
	}

	inputs = append(inputs, aliasInputs...)

	return inputs, nil
}

// prepareOutputs creates outputs for different scenarios, if no aliases were provided, new empty outputs are created from buildOptions.outputs balances.
func (e *EvilWallet) prepareOutputs(buildOptions *Options, tempWallet *Wallet) (outputs []iotago.Output,
	addrAliasMap map[string]string, tempAddresses map[string]types.Empty, err error,
) {
	if buildOptions.areOutputsProvidedWithoutAliases() {
		outputs = append(outputs, buildOptions.outputs...)
	} else {
		// if outputs were provided with aliases
		outputs, addrAliasMap, tempAddresses, err = e.matchOutputsWithAliases(buildOptions, tempWallet)
	}

	return
}

// matchInputsWithAliases gets input from the alias manager. if input was not assigned to an alias before,
// it assigns a new Fresh faucet output.
func (e *EvilWallet) matchInputsWithAliases(buildOptions *Options) (inputs []*models.Output, err error) {
	// get inputs by alias
	for inputAlias := range buildOptions.aliasInputs {
		in, ok := e.aliasManager.GetInput(inputAlias)
		if !ok {
			wallet, err2 := e.useFreshIfInputWalletNotProvided(buildOptions)
			if err2 != nil {
				err = err2
				return
			}

			// No output found for given alias, use internal Fresh output if wallets are non-empty.
			in = e.wallets.GetUnspentOutput(wallet)
			if in == nil {
				return nil, ierrors.New("could not get unspent output")
			}

			e.aliasManager.AddInputAlias(in, inputAlias)
		}
		inputs = append(inputs, in)
	}

	return inputs, nil
}

func (e *EvilWallet) useFreshIfInputWalletNotProvided(buildOptions *Options) (*Wallet, error) {
	// if input wallet is not specified, use Fresh faucet wallet
	if buildOptions.inputWallet != nil {
		return buildOptions.inputWallet, nil
	}

	// deep spam enabled and no input reuse wallet provided, use evil wallet reuse wallet if enough outputs are available
	if buildOptions.reuse {
		outputsNeeded := len(buildOptions.inputs)
		if wallet := e.wallets.reuseWallet(outputsNeeded); wallet != nil {
			return wallet, nil
		}
	}

	wallet, err := e.wallets.freshWallet()
	if err != nil {
		return nil, ierrors.Wrap(err, "no Fresh wallet is available")
	}

	return wallet, nil
}

// matchOutputsWithAliases creates outputs based on balances provided via options.
// Outputs are not yet added to the Alias Manager, as they have no ID before the transaction is created.
// Thus, they are tracker in address to alias map. If the scenario is used, the outputBatchAliases map is provided
// that indicates which outputs should be saved to the outputWallet. All other outputs are created with temporary wallet,
// and their addresses are stored in tempAddresses.
func (e *EvilWallet) matchOutputsWithAliases(buildOptions *Options, tempWallet *Wallet) (outputs []iotago.Output,
	addrAliasMap map[string]string, tempAddresses map[string]types.Empty, err error,
) {
	err = e.updateOutputBalances(buildOptions)
	if err != nil {
		return nil, nil, nil, err
	}

	tempAddresses = make(map[string]types.Empty)
	addrAliasMap = make(map[string]string)
	for alias, output := range buildOptions.aliasOutputs {
		var addr *iotago.Ed25519Address
		if _, ok := buildOptions.outputBatchAliases[alias]; ok {
			addr = buildOptions.outputWallet.Address()
		} else {
			addr = tempWallet.Address()
			tempAddresses[addr.String()] = types.Void
		}

		switch output.Type() {
		case iotago.OutputBasic:
			outputBuilder := builder.NewBasicOutputBuilder(addr, output.BaseTokenAmount())
			outputs = append(outputs, outputBuilder.MustBuild())
		case iotago.OutputAccount:
			outputBuilder := builder.NewAccountOutputBuilder(addr, addr, output.BaseTokenAmount())
			outputs = append(outputs, outputBuilder.MustBuild())
		}

		addrAliasMap[addr.String()] = alias
	}

	return
}

func (e *EvilWallet) prepareRemainderOutput(buildOptions *Options, outputs []iotago.Output) (alias string, remainderOutput iotago.Output, remainderAddress iotago.Address, added bool) {
	inputBalance := iotago.BaseToken(0)

	for inputAlias := range buildOptions.aliasInputs {
		in, _ := e.aliasManager.GetInput(inputAlias)
		inputBalance += in.Balance

		if alias == "" {
			remainderAddress = in.Address
			alias = inputAlias
		}
	}

	for _, input := range buildOptions.inputs {
		// get balance from output manager
		in := e.outputManager.GetOutput(input.OutputID)
		inputBalance += in.Balance

		if remainderAddress == nil {
			remainderAddress = in.Address
		}
	}

	outputBalance := iotago.BaseToken(0)
	for _, o := range outputs {
		outputBalance += o.BaseTokenAmount()
	}

	// remainder balances is sent to one of the address in inputs
	if outputBalance < inputBalance {
		remainderOutput = &iotago.BasicOutput{
			Amount: inputBalance - outputBalance,
			Conditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: remainderAddress},
			},
		}

		added = true
	}

	return
}

func (e *EvilWallet) updateOutputBalances(buildOptions *Options) (err error) {
	// when aliases are not used for outputs, the balance had to be provided in options, nothing to do
	if buildOptions.areOutputsProvidedWithoutAliases() {
		return
	}
	totalBalance := iotago.BaseToken(0)
	if !buildOptions.isBalanceProvided() {
		if buildOptions.areInputsProvidedWithoutAliases() {
			for _, input := range buildOptions.inputs {
				// get balance from output manager
				inputDetails := e.outputManager.GetOutput(input.OutputID)
				totalBalance += inputDetails.Balance
			}
		} else {
			for inputAlias := range buildOptions.aliasInputs {
				in, ok := e.aliasManager.GetInput(inputAlias)
				if !ok {
					err = ierrors.New("could not get input by input alias")
					return
				}
				totalBalance += in.Balance
			}
		}
		balances := utils.SplitBalanceEqually(len(buildOptions.outputs)+len(buildOptions.aliasOutputs), totalBalance)
		i := 0
		for out, output := range buildOptions.aliasOutputs {
			switch output.Type() {
			case iotago.OutputBasic:
				buildOptions.aliasOutputs[out] = &iotago.BasicOutput{
					Amount: balances[i],
				}
			case iotago.OutputAccount:
				buildOptions.aliasOutputs[out] = &iotago.AccountOutput{
					Amount: balances[i],
				}
			}
			i++
		}
	}

	return
}

func (e *EvilWallet) makeTransaction(inputs []*models.Output, outputs iotago.Outputs[iotago.Output], w *Wallet, congestionResponse *apimodels.CongestionResponse, issuerAccountID iotago.AccountID) (tx *iotago.SignedTransaction, err error) {
	clt := e.Connector().GetClient()
	currentTime := time.Now()
	targetSlot := clt.LatestAPI().TimeProvider().SlotFromTime(currentTime)
	targetAPI := clt.APIForSlot(targetSlot)

	txBuilder := builder.NewTransactionBuilder(targetAPI)

	for _, input := range inputs {
		txBuilder.AddInput(&builder.TxInput{UnlockTarget: input.Address, InputID: input.OutputID, Input: input.OutputStruct})
	}

	for _, output := range outputs {
		txBuilder.AddOutput(output)
	}

	randomPayload := tpkg.Rand12ByteArray()
	txBuilder.AddTaggedDataPayload(&iotago.TaggedData{Tag: randomPayload[:], Data: randomPayload[:]})

	walletKeys := make([]iotago.AddressKeys, len(inputs))
	for i, input := range inputs {
		addr := input.Address
		var wallet *Wallet
		if w == nil { // aliases provided with inputs, use wallet saved in outputManager
			wallet = e.outputManager.OutputIDWalletMap(input.OutputID.ToHex())
		} else {
			wallet = w
		}
		index := wallet.AddrIndexMap(addr.String())
		inputPrivateKey, _ := wallet.KeyPair(index)
		walletKeys[i] = iotago.AddressKeys{Address: addr, Keys: inputPrivateKey}
	}

	txBuilder.SetCreationSlot(targetSlot)
	// no allotment strategy
	if congestionResponse == nil {
		txBuilder.AllotAllMana(targetSlot, issuerAccountID)

		return txBuilder.Build(iotago.NewInMemoryAddressSigner(walletKeys...))
	}

	return txBuilder.Build(iotago.NewInMemoryAddressSigner(walletKeys...))
}

func (e *EvilWallet) PrepareCustomConflictsSpam(scenario *EvilScenario, strategy *models.IssuancePaymentStrategy) (txsData [][]*models.PayloadIssuanceData, allAliases ScenarioAlias, err error) {
	conflicts, allAliases := e.prepareConflictSliceForScenario(scenario, strategy)
	txsData, err = e.PrepareCustomConflicts(conflicts)

	return txsData, allAliases, err
}

func (e *EvilWallet) PrepareAccountSpam(scenario *EvilScenario, strategy *models.IssuancePaymentStrategy) (*models.PayloadIssuanceData, ScenarioAlias, error) {
	accountSpamOptions, allAliases := e.prepareFlatOptionsForAccountScenario(scenario, strategy)

	txData, err := e.CreateTransaction(accountSpamOptions...)

	return txData, allAliases, err
}

func (e *EvilWallet) evaluateIssuanceStrategy(strategy *models.IssuancePaymentStrategy) (models.AllotmentStrategy, iotago.AccountID) {
	var issuerAccountID iotago.AccountID
	if strategy.AllotmentStrategy != models.AllotmentStrategyNone {
		// get issuer accountID
		accData, err := e.accWallet.GetAccount(strategy.IssuerAlias)
		if err != nil {
			panic("could not get issuer accountID while preparing conflicts")
		}

		issuerAccountID = accData.Account.ID()
	}

	return strategy.AllotmentStrategy, issuerAccountID
}

func (e *EvilWallet) prepareConflictSliceForScenario(scenario *EvilScenario, strategy *models.IssuancePaymentStrategy) (conflictSlice []ConflictSlice, allAliases ScenarioAlias) {
	genOutputOptions := func(aliases []string) []*OutputOption {
		outputOptions := make([]*OutputOption, 0)
		for _, o := range aliases {
			outputOptions = append(outputOptions, &OutputOption{aliasName: o, outputType: iotago.OutputBasic})
		}

		return outputOptions
	}

	// make conflictSlice
	prefixedBatch, allAliases, batchOutputs := scenario.ConflictBatchWithPrefix()
	conflictSlice = make([]ConflictSlice, 0)
	for _, conflictMap := range prefixedBatch {
		conflicts := make([][]Option, 0)
		for _, aliases := range conflictMap {
			outs := genOutputOptions(aliases.Outputs)
			option := []Option{WithInputs(aliases.Inputs), WithOutputs(outs), WithOutputBatchAliases(batchOutputs)}
			if scenario.OutputWallet != nil {
				option = append(option, WithOutputWallet(scenario.OutputWallet))
			}
			if scenario.RestrictedInputWallet != nil {
				option = append(option, WithInputWallet(scenario.RestrictedInputWallet))
			}
			if scenario.Reuse {
				option = append(option, WithReuseOutputs())
			}
			option = append(option, WithIssuanceStrategy(e.evaluateIssuanceStrategy(strategy)))
			conflicts = append(conflicts, option)
		}
		conflictSlice = append(conflictSlice, conflicts)
	}

	return
}

func (e *EvilWallet) prepareFlatOptionsForAccountScenario(scenario *EvilScenario, strategy *models.IssuancePaymentStrategy) ([]Option, ScenarioAlias) {
	// we do not care about batchedOutputs, because we do not support saving account spam result in evil wallet for now
	prefixedBatch, allAliases, _ := scenario.ConflictBatchWithPrefix()
	if len(prefixedBatch) != 1 {
		panic("invalid scenario, cannot prepare flat option structure with deep scenario, EvilBatch should have only one element")
	}
	evilBatch := prefixedBatch[0]
	if len(evilBatch) != 1 {
		panic("invalid scenario, cannot prepare flat option structure with deep scenario, EvilBatch should have only one element")
	}

	genOutputOptions := func(aliases []string) []*OutputOption {
		outputOptions := make([]*OutputOption, 0)
		for _, o := range aliases {
			outputOptions = append(outputOptions, &OutputOption{
				aliasName:  o,
				outputType: iotago.OutputAccount,
			})
		}

		return outputOptions
	}
	scenarioAlias := evilBatch[0]
	outs := genOutputOptions(scenarioAlias.Outputs)

	return []Option{
		WithInputs(scenarioAlias.Inputs),
		WithOutputs(outs),
		WithIssuanceStrategy(e.evaluateIssuanceStrategy(strategy)),
	}, allAliases
}

// SetTxOutputsSolid marks all outputs as solid in OutputManager for clientID.
func (e *EvilWallet) SetTxOutputsSolid(outputs iotago.OutputIDs, clientID string) {
	for _, out := range outputs {
		e.outputManager.SetOutputIDSolidForIssuer(out, clientID)
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

func WithClients(urls ...string) options.Option[EvilWallet] {
	return func(opts *EvilWallet) {
		opts.optsClientURLs = urls
	}
}

func WithAccountsWallet(wallet *accountwallet.AccountWallet) options.Option[EvilWallet] {
	return func(opts *EvilWallet) {
		opts.accWallet = wallet
	}
}
