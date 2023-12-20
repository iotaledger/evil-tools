package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/evil-tools/pkg/evilwallet"
	"github.com/iotaledger/evil-tools/pkg/spammer"
	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/app/configuration"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
)

func parseFlags() (string, bool) {
	if len(os.Args) <= 1 {
		return ScriptSpammer, true
	}
	script := os.Args[1]

	loggerRoot.LogInfof("script %s", script)

	switch script {
	case ScriptSpammer:
		parseBasicSpamFlags()
	case ScriptAccounts:
		// pass subcommands
		subcommands := make([]string, 0)
		if len(os.Args) > 2 {
			subcommands = os.Args[2:]
		}
		splitedCmds := readSubcommandsAndFlagSets(subcommands)
		accountsSubcommandsFlags = parseAccountTestFlags(splitedCmds)

	}
	if script == "help" || script == "-h" || script == "--help" {
		return script, true
	}

	return script, false
}

func parseOptionFlagSet(flagSet *flag.FlagSet, args ...[]string) {
	commands := os.Args[2:]
	if len(args) > 0 {
		commands = args[0]
	}
	err := flagSet.Parse(commands)
	if err != nil {
		loggerRoot.LogError("Cannot parse first `script` parameter")
		return
	}
}

func initRootLoggerFromConfig() log.Logger {
	config := configuration.New()

	loggerConfig := &app.LoggerConfig{
		Name: "",
		OutputPaths: []string{
			"stdout",
			"evil-spammer.log",
		},
	}
	config.BindParameters(flagSetLogger, "logger", loggerConfig)

	// parse the flagSet
	configuration.ParseFlagSets([]*flag.FlagSet{flagSetLogger})

	// initialize the root logger
	loggerRoot, err := app.NewLoggerFromConfig(loggerConfig)
	if err != nil {
		panic(err)
	}

	return loggerRoot
}

func parseBasicSpamFlags() {
	urls := flagSetOption.String("urls", "", "API urls for clients used in test separated with commas")
	spamType := flagSetOption.String("spammer", "", "Spammers used during test. Format: strings separated with comma, available options: 'blk' - block,"+
		" 'tx' - transaction, 'ds' - double spends spammers, 'nds' - n-spends spammer, 'custom' - spams with provided scenario, 'bb' - blowball")
	rate := flagSetOption.Int("rate", customSpamParams.Rate, "Spamming rate for provided 'spammer'. Format: numbers separated with comma, e.g. 10,100,1 if three spammers were provided for 'spammer' parameter.")
	duration := flagSetOption.String("duration", "", "Spam duration. If not provided spam will lats infinitely. Format: separated by commas list of decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.\n Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'.")
	timeunit := flagSetOption.Duration("unit", customSpamParams.TimeUnit, "Time unit for the spamming rate. Format: decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.\n Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'.")
	delayBetweenConflicts := flagSetOption.Duration("dbc", customSpamParams.DelayBetweenConflicts, "delayBetweenConflicts - Time delay between conflicts in double spend spamming")
	scenario := flagSetOption.String("scenario", "", "Name of the EvilBatch that should be used for the spam. By default uses Scenario1. Possible scenarios can be found in evilwallet/customscenarion.go.")
	deepSpam := flagSetOption.Bool("deep", customSpamParams.DeepSpam, "Enable the deep spam, by reusing outputs created during the spam. To enable provide an empty flag.")
	nSpend := flagSetOption.Int("nSpend", customSpamParams.NSpend, "Number of outputs to be spent in n-spends spammer for the spammer type needs to be set to 'ds'. Default value is 2 for double-spend.")
	account := flagSetOption.String("account", "", "Account alias to be used for the spam. Account should be created first with accounts tool.")

	parseOptionFlagSet(flagSetOption)

	if *urls != "" {
		parsedUrls := parseCommaSepString(*urls)
		customSpamParams.ClientURLs = parsedUrls
	}
	customSpamParams.SpamType = *spamType
	customSpamParams.Rate = *rate
	if *duration != "" {
		customSpamParams.Duration, _ = time.ParseDuration(*duration)
	} else {
		customSpamParams.Duration = spammer.InfiniteDuration
	}
	if *scenario != "" {
		conflictBatch, ok := evilwallet.GetScenario(*scenario)
		if ok {
			customSpamParams.Scenario = conflictBatch
			customSpamParams.ScenarioName = *scenario
		}
	}

	customSpamParams.NSpend = *nSpend
	customSpamParams.DeepSpam = *deepSpam
	customSpamParams.TimeUnit = *timeunit
	customSpamParams.DelayBetweenConflicts = *delayBetweenConflicts
	if *account != "" {
		customSpamParams.AccountAlias = *account
	}
}

// readSubcommandsAndFlagSets splits the subcommands on multiple flag sets.
func readSubcommandsAndFlagSets(subcommands []string) [][]string {
	prevSplitIndex := 0
	subcommandsSplit := make([][]string, 0)
	if len(subcommands) == 0 {
		return nil
	}

	// mainCmd := make([]string, 0)
	for index := 0; index < len(subcommands); index++ {
		validCommand := accountwallet.AvailableCommands(subcommands[index])

		if !strings.HasPrefix(subcommands[index], "--") && validCommand {
			if index != 0 {
				subcommandsSplit = append(subcommandsSplit, subcommands[prevSplitIndex:index])
			}
			prevSplitIndex = index
		}
	}
	subcommandsSplit = append(subcommandsSplit, subcommands[prevSplitIndex:])

	return subcommandsSplit
}

func parseAccountTestFlags(splitedCmds [][]string) []accountwallet.AccountSubcommands {
	parsedCmds := make([]accountwallet.AccountSubcommands, 0)

	for _, cmds := range splitedCmds {
		switch cmds[0] {
		case "create":
			createAccountParams, err := parseCreateAccountFlags(cmds[1:])
			if err != nil {
				continue
			}

			parsedCmds = append(parsedCmds, createAccountParams)
		case "convert":
			convertAccountParams, err := parseConvertAccountFlags(cmds[1:])
			if err != nil {
				continue
			}

			parsedCmds = append(parsedCmds, convertAccountParams)
		case "destroy":
			destroyAccountParams, err := parseDestroyAccountFlags(cmds[1:])
			if err != nil {
				continue
			}

			parsedCmds = append(parsedCmds, destroyAccountParams)
		case "allot":
			allotAccountParams, err := parseAllotAccountFlags(cmds[1:])
			if err != nil {
				continue
			}

			parsedCmds = append(parsedCmds, allotAccountParams)
		case "delegate":
			delegatingAccountParams, err := parseDelegateAccountFlags(cmds[1:])
			if err != nil {
				continue
			}

			parsedCmds = append(parsedCmds, delegatingAccountParams)
		case "stake":
			stakingAccountParams, err := parseStakeAccountFlags(cmds[1:])
			if err != nil {
				continue
			}

			parsedCmds = append(parsedCmds, stakingAccountParams)
		case "update":
			updateAccountParams, err := parseUpdateAccountFlags(cmds[1:])
			if err != nil {
				continue
			}

			parsedCmds = append(parsedCmds, updateAccountParams)
		case "list":
			parsedCmds = append(parsedCmds, &accountwallet.NoAccountParams{
				Operation: accountwallet.OperationListAccounts,
			})
		default:
			accountUsage()
			return nil
		}
	}

	return parsedCmds
}

func accountUsage() {
	fmt.Println("Usage for accounts [COMMAND] [FLAGS], multiple commands can be chained together.")
	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameCreateAccount)
	_, _ = parseCreateAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameConvertAccount)
	_, _ = parseConvertAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameDestroyAccount)
	_, _ = parseDestroyAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameAllotAccount)
	_, _ = parseAllotAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameDelegateAccount)
	_, _ = parseDelegateAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameStakeAccount)
	_, _ = parseStakeAccountFlags(nil)
}

func parseCreateAccountFlags(subcommands []string) (*accountwallet.CreateAccountParams, error) {
	alias := flagSetCreate.String("alias", "", "The alias name of new created account")
	noBif := flagSetCreate.Bool("noBIF", false, "Create account without Block Issuer Feature, can only be set false no if implicit is false, as each account created implicitly needs to have BIF.")
	implicit := flagSetCreate.Bool("implicit", false, "Create an implicit account")
	noTransition := flagSetCreate.Bool("noTransition", false, "account should not be transitioned to a full account if created with implicit address. Transition enabled by default, to disable provide an empty flag.")

	if subcommands == nil {
		flagSetCreate.Usage()

		return nil, ierrors.Errorf("no subcommands")
	}

	loggerRoot.LogInfof("Parsing create account flags, subcommands: %v", subcommands)
	err := flagSetCreate.Parse(subcommands)
	if err != nil {
		loggerRoot.LogError("Cannot parse first `script` parameter")

		return nil, ierrors.Wrap(err, "cannot parse first `script` parameter")
	}

	if *implicit && *noBif {
		loggerRoot.LogInfo("WARN: Implicit account cannot be created without Block Issuance Feature, flag noBIF will be ignored")
		*noBif = false
	}

	loggerRoot.LogInfof("Parsed flags: alias: %s, BIF: %t, implicit: %t, transition: %t", *alias, *noBif, *implicit, *noTransition)

	if !*implicit == !*noTransition {
		loggerRoot.LogInfo("WARN: Implicit flag set to false, account will be created non-implicitly by Faucet, no need for transition, flag will be ignored")
		*noTransition = true
	}

	return &accountwallet.CreateAccountParams{
		Alias:      *alias,
		NoBIF:      *noBif,
		Implicit:   *implicit,
		Transition: !*noTransition,
	}, nil
}

func parseConvertAccountFlags(subcommands []string) (*accountwallet.ConvertAccountParams, error) {
	alias := flagSetConvert.String("alias", "", "The implicit account to be converted to full account with BIF.")

	if subcommands == nil {
		flagSetConvert.Usage()

		return nil, ierrors.Errorf("no subcommands")
	}

	loggerRoot.LogInfof("Parsing convert account flags, subcommands: %v", subcommands)
	err := flagSetConvert.Parse(subcommands)
	if err != nil {
		loggerRoot.LogError("Cannot parse first `script` parameter")

		return nil, ierrors.Wrap(err, "cannot parse first `script` parameter")
	}

	return &accountwallet.ConvertAccountParams{
		AccountAlias: *alias,
	}, nil
}

func parseDestroyAccountFlags(subcommands []string) (*accountwallet.DestroyAccountParams, error) {
	alias := flagSetDestroy.String("alias", "", "The alias name of the account to be destroyed")
	expirySlot := flagSetDestroy.Int64("expirySlot", 0, "The expiry slot of the account to be destroyed")

	if subcommands == nil {
		flagSetDestroy.Usage()

		return nil, ierrors.Errorf("no subcommands")
	}

	loggerRoot.LogInfof("Parsing destroy account flags, subcommands: %v", subcommands)
	err := flagSetDestroy.Parse(subcommands)
	if err != nil {
		loggerRoot.LogError("Cannot parse first `script` parameter")

		return nil, ierrors.Wrap(err, "cannot parse first `script` parameter")
	}

	return &accountwallet.DestroyAccountParams{
		AccountAlias: *alias,
		ExpirySlot:   uint64(*expirySlot),
	}, nil
}

func parseAllotAccountFlags(subcommands []string) (*accountwallet.AllotAccountParams, error) {
	from := flagSetAllot.String("from", "", "The alias name of the account to allot mana from")
	to := flagSetAllot.String("to", "", "The alias of the account to allot mana to")
	amount := flagSetAllot.Int64("amount", 1000, "The amount of mana to allot")

	if subcommands == nil {
		flagSetAllot.Usage()

		return nil, ierrors.Errorf("no subcommands")
	}

	loggerRoot.LogInfof("Parsing allot account flags, subcommands: %v", subcommands)
	err := flagSetAllot.Parse(subcommands)
	if err != nil {
		loggerRoot.LogErrorf("Cannot parse first `script` parameter")

		return nil, ierrors.Wrap(err, "cannot parse first `script` parameter")
	}

	return &accountwallet.AllotAccountParams{
		From:   *from,
		To:     *to,
		Amount: uint64(*amount),
	}, nil
}

func parseStakeAccountFlags(subcommands []string) (*accountwallet.StakeAccountParams, error) {
	alias := flagSetStake.String("alias", "", "The alias name of the account to stake")
	amount := flagSetStake.Int64("amount", 100, "The amount of tokens to stake")
	fixedCost := flagSetStake.Int64("fixedCost", 0, "The fixed cost of the account to stake")
	startEpoch := flagSetStake.Int64("startEpoch", 0, "The start epoch of the account to stake")
	endEpoch := flagSetStake.Int64("endEpoch", 0, "The end epoch of the account to stake")

	if subcommands == nil {
		flagSetStake.Usage()

		return nil, ierrors.Errorf("no subcommands")
	}

	loggerRoot.LogInfof("Parsing staking account flags, subcommands: %v", subcommands)
	err := flagSetStake.Parse(subcommands)
	if err != nil {
		loggerRoot.LogError("Cannot parse first `script` parameter")

		return nil, ierrors.Wrap(err, "cannot parse first `script` parameter")
	}

	return &accountwallet.StakeAccountParams{
		Alias:      *alias,
		Amount:     uint64(*amount),
		FixedCost:  uint64(*fixedCost),
		StartEpoch: uint64(*startEpoch),
		EndEpoch:   uint64(*endEpoch),
	}, nil
}

func parseDelegateAccountFlags(subcommands []string) (*accountwallet.DelegateAccountParams, error) {
	from := flagSetDelegate.String("from", "", "The alias name of the account to delegate mana from")
	to := flagSetDelegate.String("to", "", "The alias of the account to delegate mana to")
	amount := flagSetDelegate.Int64("amount", 100, "The amount of mana to delegate")

	if subcommands == nil {
		flagSetDelegate.Usage()

		return nil, ierrors.Errorf("no subcommands")
	}

	loggerRoot.LogInfof("Parsing delegate account flags, subcommands: %v", subcommands)
	err := flagSetDelegate.Parse(subcommands)
	if err != nil {
		loggerRoot.LogError("Cannot parse first `script` parameter")

		return nil, ierrors.Wrap(err, "cannot parse first `script` parameter")
	}

	return &accountwallet.DelegateAccountParams{
		From:   *from,
		To:     *to,
		Amount: uint64(*amount),
	}, nil
}

func parseUpdateAccountFlags(subcommands []string) (*accountwallet.UpdateAccountParams, error) {
	alias := flagSetUpdate.String("alias", "", "The alias name of the account to update")
	bik := flagSetUpdate.String("bik", "", "The block issuer key (in hex) to add")
	amount := flagSetUpdate.Int64("addamount", 100, "The amount of token to add")
	mana := flagSetUpdate.Int64("addmana", 100, "The amount of mana to add")
	expirySlot := flagSetUpdate.Int64("expirySlot", 0, "Update the expiry slot of the account")

	if subcommands == nil {
		flagSetUpdate.Usage()

		return nil, ierrors.Errorf("no subcommands")
	}

	loggerRoot.LogInfof("Parsing update account flags, subcommands: %v", subcommands)
	err := flagSetUpdate.Parse(subcommands)
	if err != nil {
		loggerRoot.LogError("Cannot parse first `script` parameter")

		return nil, ierrors.Wrap(err, "cannot parse first `script` parameter")
	}

	return &accountwallet.UpdateAccountParams{
		Alias:          *alias,
		BlockIssuerKey: *bik,
		Amount:         uint64(*amount),
		Mana:           uint64(*mana),
		ExpirySlot:     uint64(*expirySlot),
	}, nil
}

func parseCommaSepString(urls string) []string {
	split := strings.Split(urls, ",")

	return split
}
