package evilwallet

import (
	"context"
	"time"

	"github.com/iotaledger/evil-tools/pkg/accountmanager"
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/iotaledger/iota.go/v4/wallet"
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
	defaultClientsURLs = []string{"http://localhost:8050"}
	defaultFaucetURL   = "http://localhost:8088"

	NoFreshOutputsAvailable = ierrors.New("no fresh wallet is available")
)

// region EvilWallet ///////////////////////////////////////////////////////////////////////////////////////////////////////

// EvilWallet provides a user-friendly way to do complicated double spend scenarios.
type EvilWallet struct {
	log.Logger

	wallets       *Wallets
	accManager    *accountmanager.Manager
	connector     models.Connector
	outputManager *OutputManager
	aliasManager  *AliasManager

	minOutputStorageDeposit iotago.BaseToken

	optsClientURLs []string
	optsFaucetURL  string
}

// NewEvilWallet creates an EvilWallet instance.
func NewEvilWallet(logger log.Logger, opts ...options.Option[EvilWallet]) *EvilWallet {
	return options.Apply(&EvilWallet{
		Logger:                  logger.NewChildLogger("EvilWallet"),
		wallets:                 NewWallets(),
		aliasManager:            NewAliasManager(),
		minOutputStorageDeposit: MinOutputStorageDeposit,
		optsClientURLs:          defaultClientsURLs,
		optsFaucetURL:           defaultFaucetURL,
	}, opts, func(w *EvilWallet) {
		connector := lo.PanicOnErr(models.NewWebClients(w.optsClientURLs, w.optsFaucetURL))
		w.connector = connector
		w.outputManager = NewOutputManager(connector, w.wallets, w.Logger.NewChildLogger("OutputManager"))

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

func (e *EvilWallet) GetAccount(ctx context.Context, alias string) (wallet.Account, error) {
	account, err := e.accManager.GetReadyAccount(ctx, e.connector.GetClient(), alias)
	if err != nil {
		return nil, err
	}

	return account.Account, nil
}

// CreateBlock creates a block with the
// update wallet with newly created output freshly requested Congestion and Issuance data.
func (e *EvilWallet) CreateBlock(ctx context.Context, clt models.Client, payload iotago.Payload, issuer wallet.Account, strongParents ...iotago.BlockID) (*iotago.Block, error) {
	congestionResp, issuerResp, version, err := e.accManager.RequestBlockIssuanceData(ctx, clt, issuer)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get block built data")
	}

	block, err := e.accManager.CreateBlock(e.connector.GetClient(), payload, issuer, congestionResp, issuerResp, version, strongParents...)
	if err != nil {
		return nil, err
	}

	return block, nil
}

func (e *EvilWallet) PrepareAndPostBlockWithPayload(ctx context.Context, clt models.Client, payload iotago.Payload, issuer wallet.Account) (iotago.BlockID, error) {
	congestionResp, issuerResp, version, err := e.accManager.RequestBlockIssuanceData(ctx, clt, issuer)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrap(err, "failed to get block built data")
	}
	blockID, err := e.accManager.PostWithBlock(ctx, clt, payload, issuer, congestionResp, issuerResp, version)
	if err != nil {
		return iotago.EmptyBlockID, err
	}

	return blockID, nil
}

func (e *EvilWallet) PrepareAndPostBlockWithTxBuildData(ctx context.Context, clt models.Client, txBuilder *builder.TransactionBuilder, signingKeys []iotago.AddressKeys, issuer wallet.Account) (iotago.BlockID, *iotago.Transaction, error) {
	congestionResp, issuerResp, version, err := e.accManager.RequestBlockIssuanceData(ctx, clt, issuer)
	if err != nil {
		return iotago.EmptyBlockID, nil, ierrors.Wrap(err, "failed to get block built data")
	}

	// handle allotment strategy
	txBuilder.AllotAllMana(txBuilder.CreationSlot(), issuer.ID(), 0)
	signedTx, err := txBuilder.Build(iotago.NewInMemoryAddressSigner(signingKeys...))
	if err != nil {
		return iotago.EmptyBlockID, nil, ierrors.Wrap(err, "failed to build and sign transaction")
	}

	txID, err := signedTx.Transaction.ID()
	if err != nil {
		return iotago.EmptyBlockID, nil, ierrors.Wrap(err, "failed to get transaction id")
	}

	err = e.setTxOutputIDs(signedTx.Transaction)
	if err != nil {
		return iotago.EmptyBlockID, nil, ierrors.Wrapf(err, "failed to set output ids for transaction %s", txID.String())
	}

	blockID, err := e.accManager.PostWithBlock(ctx, clt, signedTx, issuer, congestionResp, issuerResp, version)
	if err != nil {
		return iotago.EmptyBlockID, nil, err
	}

	return blockID, signedTx.Transaction, nil
}

func (e *EvilWallet) setTxOutputIDs(tx *iotago.Transaction) error {
	for idx, out := range tx.Outputs {
		tempID := lo.PanicOnErr(models.NewTempOutputID(e.connector.GetClient().LatestAPI(), out))
		modelOutput := e.outputManager.getOutputFromWallet(tempID)
		if modelOutput == nil {
			return ierrors.Errorf("output not found for address %s", out.UnlockConditionSet().Address().Address.String())
		}
		txID, err := tx.ID()
		if err != nil {
			return ierrors.Wrap(err, "failed to get transaction id")
		}
		outID := iotago.OutputIDFromTransactionIDAndIndex(txID, uint16(idx))
		modelOutput.OutputID = outID
	}

	return nil
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

func (e *EvilWallet) PrepareCustomConflicts(ctx context.Context, conflictsMaps []ConflictSlice) (conflictBatch [][]*models.PayloadIssuanceData, err error) {
	for _, conflictMap := range conflictsMaps {
		var txsData []*models.PayloadIssuanceData
		for _, conflictOptions := range conflictMap {
			issuanceData, err2 := e.CreateTransaction(ctx, conflictOptions...)
			if err2 != nil {
				return nil, err2
			}
			txsData = append(txsData, issuanceData)
		}
		conflictBatch = append(conflictBatch, txsData)
	}

	return conflictBatch, nil
}

// CreateTransaction creates a transaction builder based on provided options. If no input wallet is provided, the next non-empty faucet wallet is used.
// No mana allotment is done here, tx is not signed and built yet.
// Inputs of the transaction are determined in three ways:
// 1 - inputs are provided directly without associated alias, 2- provided only an alias, but inputs are stored in an alias manager,
// 3 - provided alias, but there are no inputs assigned in Alias manager, so aliases will be assigned to next ready inputs from input wallet.
func (e *EvilWallet) CreateTransaction(ctx context.Context, options ...Option) (*models.PayloadIssuanceData, error) {
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

	outputs, addrAliasMap, tempAddresses, err := e.prepareOutputs(ctx, buildOptions, tempWallet)
	if err != nil {
		return nil, err
	}

	alias, remainder, hasRemainder := e.prepareRemainderOutput(inputs, outputs)
	if hasRemainder {
		outputs = append(outputs, remainder)
		if alias != "" && addrAliasMap != nil {
			tempID := lo.PanicOnErr(models.NewTempOutputID(e.connector.GetClient().LatestAPI(), remainder))
			addrAliasMap[tempID] = alias
		}
	}
	txBuilder, signingKeys := e.prepareTransactionBuild(inputs, outputs, buildOptions.inputWallet)
	issuanceData := &models.PayloadIssuanceData{
		Type:               iotago.PayloadSignedTransaction,
		TransactionBuilder: txBuilder,
		TxSigningKeys:      signingKeys,
	}

	addedOutputs := e.addOutputsToOutputManager(outputs, buildOptions.outputWallet, tempWallet, tempAddresses)
	e.registerOutputAliases(addedOutputs, addrAliasMap)

	return issuanceData, nil
}

// addOutputsToOutputManager adds output to the OutputManager.
func (e *EvilWallet) addOutputsToOutputManager(outputs []iotago.Output, outWallet, tmpWallet *Wallet, tempAddresses map[string]types.Empty) []*models.OutputData {
	modelOutputs := make([]*models.OutputData, 0)
	for _, out := range outputs {
		if out.UnlockConditionSet().Address() == nil {
			continue
		}

		// register UnlockConditionAddress only (skip account outputs)
		addr := out.UnlockConditionSet().Address().Address

		var output *models.OutputData
		// outputs in the middle of the scenario structure are created with tempWallet,
		//only outputs that are not used in the scenario structure are added to the outWaller and can be reused.
		if _, ok := tempAddresses[addr.String()]; ok {
			output = e.outputManager.AddOutput(e.connector.GetClient().LatestAPI(), tmpWallet, out)
		} else {
			output = e.outputManager.AddOutput(e.connector.GetClient().LatestAPI(), outWallet, out)
		}

		modelOutputs = append(modelOutputs, output)
	}

	return modelOutputs
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
	// assign fresh wallet that will be used when only aliases were provided
	w, err := e.useFreshIfInputWalletNotProvided(buildOptions)
	if err != nil {
		return err
	}

	buildOptions.inputWallet = w

	return nil
}

// registerOutputAliases adds models.Output references to their aliases to the AliasManager.
func (e *EvilWallet) registerOutputAliases(outputs []*models.OutputData, idAliasMap map[models.TempOutputID]string) {
	if len(idAliasMap) == 0 {
		return
	}

	for _, out := range outputs {
		tempID := lo.PanicOnErr(models.NewTempOutputID(e.connector.GetClient().LatestAPI(), out.OutputStruct))
		// register output alias
		e.aliasManager.AddOutputAlias(out, idAliasMap[tempID])

		// register output as unspent output(input)
		e.aliasManager.AddInputAlias(out, idAliasMap[tempID])
	}
}

func (e *EvilWallet) prepareInputs(buildOptions *Options) (inputs []*models.OutputData, err error) {
	// case 1, inputs provided
	if buildOptions.areInputsProvidedWithoutAliases() {
		inputs = append(inputs, buildOptions.inputs...)

		return
	}
	// case 2, no inputs, there has to be aliases provided
	aliasInputs, err := e.matchInputsWithAliases(buildOptions)
	if err != nil {
		return nil, err
	}

	inputs = append(inputs, aliasInputs...)

	return inputs, nil
}

// prepareOutputs creates outputs for different scenarios, if no aliases were provided, new empty outputs are created from buildOptions.outputs balances.
func (e *EvilWallet) prepareOutputs(ctx context.Context, buildOptions *Options, tempWallet *Wallet) (outputs []iotago.Output,
	addrAliasMap map[models.TempOutputID]string, tempAddresses map[string]types.Empty, err error,
) {
	if buildOptions.areOutputsProvidedWithoutAliases() {
		outputs = append(outputs, buildOptions.outputs...)
	} else {
		// if outputs were provided with aliases
		outputs, addrAliasMap, tempAddresses, err = e.matchOutputsWithAliases(ctx, buildOptions, tempWallet)
	}

	return
}

// matchInputsWithAliases gets input from the alias manager. if input was not assigned to an alias before,
// it assigns a new Fresh faucet output.
func (e *EvilWallet) matchInputsWithAliases(buildOptions *Options) (inputs []*models.OutputData, err error) {
	// get inputs by alias
	for inputAlias := range buildOptions.aliasInputs {
		in, ok := e.aliasManager.GetInput(inputAlias)
		if !ok {
			// No output found for given alias, use internal Fresh output if wallets are non-empty.
			in = buildOptions.inputWallet.GetUnspentOutput()
			if in == nil {
				return nil, NoFreshOutputsAvailable
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
		if w := e.wallets.reuseWallet(outputsNeeded); w != nil {
			return w, nil
		}
	}

	w, err := e.wallets.freshWallet()
	if err != nil {
		return nil, ierrors.Wrap(err, "no Fresh wallet is available")
	}

	return w, nil
}

// matchOutputsWithAliases creates outputs based on balances provided via options.
// Outputs are not yet added to the Alias Manager, as they have no ID before the transaction is created.
// Thus, they are tracker in address to alias map. If the scenario is used, the outputBatchAliases map is provided
// that indicates which outputs should be saved to the outputWallet. All other outputs are created with temporary wallet,
// and their addresses are stored in tempAddresses.
func (e *EvilWallet) matchOutputsWithAliases(ctx context.Context, buildOptions *Options, tempWallet *Wallet) (outputs []iotago.Output,
	idAliasMap map[models.TempOutputID]string, tempAddresses map[string]types.Empty, err error,
) {
	err = e.updateOutputBalances(ctx, buildOptions)
	if err != nil {
		return nil, nil, nil, err
	}

	tempAddresses = make(map[string]types.Empty)
	idAliasMap = make(map[models.TempOutputID]string)
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
			outputBuilder := builder.NewBasicOutputBuilder(addr, output.BaseTokenAmount()).
				Mana(output.StoredMana())
			outputs = append(outputs, outputBuilder.MustBuild())
		case iotago.OutputAccount:
			outputBuilder := builder.NewAccountOutputBuilder(addr, output.BaseTokenAmount())
			outputs = append(outputs, outputBuilder.MustBuild())
		}
		tempID := lo.PanicOnErr(models.NewTempOutputID(e.connector.GetClient().LatestAPI(), output))
		idAliasMap[tempID] = alias
	}

	return
}

func (e *EvilWallet) prepareRemainderOutput(inputs []*models.OutputData, outputs []iotago.Output) (alias string, remainderOutput iotago.Output, added bool) {
	inputBalance := iotago.BaseToken(0)

	var remainderAddress iotago.Address
	for _, input := range inputs {
		inputBalance += input.OutputStruct.BaseTokenAmount()
		remainderAddress = input.Address
	}

	outputBalance := iotago.BaseToken(0)
	for _, o := range outputs {
		outputBalance += o.BaseTokenAmount()
	}

	// remainder balances is sent to one of the address in inputs, because it's too late to update output,
	// and we cannot have two outputs for the same address in the evil spammer
	if outputBalance < inputBalance {
		remainderOutput = &iotago.BasicOutput{
			Amount: inputBalance - outputBalance,
			UnlockConditions: iotago.BasicOutputUnlockConditions{
				&iotago.AddressUnlockCondition{Address: remainderAddress},
			},
		}

		added = true
	}

	return
}

func (e *EvilWallet) updateOutputBalances(ctx context.Context, buildOptions *Options) (err error) {
	// when aliases are not used for outputs, the balance had to be provided in options, nothing to do
	if buildOptions.areOutputsProvidedWithoutAliases() {
		return
	}
	totalBalance := iotago.BaseToken(0)
	if !buildOptions.isBalanceProvided() {
		if buildOptions.areInputsProvidedWithoutAliases() {
			for _, input := range buildOptions.inputs {
				// get balance from output manager
				tempID := lo.PanicOnErr(models.NewTempOutputID(e.connector.GetClient().LatestAPI(), input.OutputStruct))
				inputDetails := e.outputManager.GetOutput(ctx, tempID, input.OutputID)
				totalBalance += inputDetails.OutputStruct.BaseTokenAmount()
			}
		} else {
			for inputAlias := range buildOptions.aliasInputs {
				in, ok := e.aliasManager.GetInput(inputAlias)
				if !ok {
					err = ierrors.New("could not get input by input alias")
					return
				}
				totalBalance += in.OutputStruct.BaseTokenAmount()
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

func (e *EvilWallet) prepareTransactionBuild(inputs []*models.OutputData, outputs iotago.Outputs[iotago.Output], w *Wallet) (tx *builder.TransactionBuilder, keys []iotago.AddressKeys) {
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
		var wlt *Wallet
		if w == nil { // aliases provided with inputs, use wallet saved in the outputManager
			tempID := lo.PanicOnErr(models.NewTempOutputID(e.connector.GetClient().LatestAPI(), input.OutputStruct))
			wlt = e.outputManager.TempIDWalletMap(tempID)
		} else {
			wlt = w
		}
		index := wlt.AddrIndexMap(addr.String())
		inputPrivateKey, _ := wlt.KeyPair(index)
		walletKeys[i] = iotago.AddressKeys{Address: addr, Keys: inputPrivateKey}
	}

	txBuilder.SetCreationSlot(targetSlot)

	return txBuilder, walletKeys
}

func (e *EvilWallet) PrepareCustomConflictsSpam(ctx context.Context, scenario *EvilScenario) (txsData [][]*models.PayloadIssuanceData, allAliases ScenarioAlias, err error) {
	conflicts, allAliases := e.prepareConflictSliceForScenario(scenario)
	txsData, err = e.PrepareCustomConflicts(ctx, conflicts)

	return txsData, allAliases, err
}

func (e *EvilWallet) PrepareAccountSpam(ctx context.Context, scenario *EvilScenario) (*models.PayloadIssuanceData, ScenarioAlias, error) {
	accountSpamOptions, allAliases := e.prepareFlatOptionsForAccountScenario(scenario)

	issuanceData, err := e.CreateTransaction(ctx, accountSpamOptions...)

	return issuanceData, allAliases, err
}

func (e *EvilWallet) prepareConflictSliceForScenario(scenario *EvilScenario) (conflictSlice []ConflictSlice, allAliases ScenarioAlias) {
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

			conflicts = append(conflicts, option)
		}
		conflictSlice = append(conflictSlice, conflicts)
	}

	return
}

func (e *EvilWallet) prepareFlatOptionsForAccountScenario(scenario *EvilScenario) ([]Option, ScenarioAlias) {
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

func WithAccountsManager(manager *accountmanager.Manager) options.Option[EvilWallet] {
	return func(opts *EvilWallet) {
		opts.accManager = manager
	}
}
