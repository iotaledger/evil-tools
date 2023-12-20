package main

import (
	"context"
	"fmt"

	flag "github.com/spf13/pflag"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/evil-tools/pkg/interactive"
	"github.com/iotaledger/evil-tools/programs"
	"github.com/iotaledger/hive.go/app/configuration"
	"github.com/iotaledger/hive.go/log"
)

var (
	loggerRoot log.Logger

	flagSetLogger   = configuration.NewUnsortedFlagSet("logger", flag.ContinueOnError)
	flagSetOption   = configuration.NewUnsortedFlagSet("script flag set", flag.ExitOnError)
	flagSetCreate   = configuration.NewUnsortedFlagSet("create", flag.ExitOnError)
	flagSetConvert  = configuration.NewUnsortedFlagSet("convert", flag.ExitOnError)
	flagSetDestroy  = configuration.NewUnsortedFlagSet("destroy", flag.ExitOnError)
	flagSetAllot    = configuration.NewUnsortedFlagSet("allot", flag.ExitOnError)
	flagSetStake    = configuration.NewUnsortedFlagSet("stake", flag.ExitOnError)
	flagSetDelegate = configuration.NewUnsortedFlagSet("delegate", flag.ExitOnError)
	flagSetUpdate   = configuration.NewUnsortedFlagSet("update", flag.ExitOnError)
)

const (
	ScriptInteractive = "interactive"
	ScriptSpammer     = "spammer"
	ScriptAccounts    = "accounts"
)

func main() {
	// init logger
	loggerRoot = initRootLoggerFromConfig()

	script, help := parseFlags()
	if help {
		fmt.Printf("Usage of the Evil Spammer tool, provide the first argument for the selected mode:\n"+
			"'%s' - enters the interactive mode.\n"+
			"'%s' - can be parametrized with additional flags to run one time spammer. Run 'evil-wallet basic -h' for the list of possible flags.\n"+
			"'%s' - tool for account creation and transition. Run 'evil-wallet accounts -h' for the list of possible flags.\n",
			ScriptInteractive, ScriptSpammer, ScriptAccounts)

		return
	}

	// init account wallet
	var accWallet *accountwallet.AccountWallet
	var err error
	//nolint:all,goconst
	if script == ScriptSpammer || script == ScriptAccounts {
		// read config here
		config := accountwallet.LoadConfiguration()
		// load wallet
		accWallet, err = accountwallet.Run(loggerRoot, config)
		if err != nil {
			loggerRoot.LogError(err.Error())
			loggerRoot.LogError("Failed to init account wallet, exitting...")

			return
		}

		// save wallet and latest faucet output
		defer func() {
			err = accountwallet.SaveState(accWallet)
			if err != nil {
				loggerRoot.LogErrorf("Error while saving wallet state: %v", err)
			}
			accountwallet.SaveConfiguration(config)

		}()
	}

	// run selected test scenario
	ctx := context.Background()
	switch script {
	case ScriptInteractive:
		interactive.Run(ctx, loggerRoot)
	case ScriptSpammer:
		dispatcher := programs.NewDispatcher(accWallet)
		dispatcher.RunSpam(ctx, loggerRoot, &customSpamParams)
	case ScriptAccounts:
		accountsSubcommands(ctx, loggerRoot, accWallet, accountsSubcommandsFlags)
	default:
		loggerRoot.LogWarn("Unknown parameter for script, possible values: interactive, spammer, accounts")
	}
}

func accountsSubcommands(ctx context.Context, logger log.Logger, wallet *accountwallet.AccountWallet, subcommands []accountwallet.AccountSubcommands) {
	for _, sub := range subcommands {
		accountsSubcommand(ctx, logger, wallet, sub)
	}
}

//nolint:all,forcetypassert
func accountsSubcommand(ctx context.Context, logger log.Logger, wallet *accountwallet.AccountWallet, sub accountwallet.AccountSubcommands) {
	switch sub.Type() {
	case accountwallet.OperationCreateAccount:
		logger.LogInfof("Run subcommand: %s, with parametetr set: %v", accountwallet.OperationCreateAccount.String(), sub)
		params, ok := sub.(*accountwallet.CreateAccountParams)
		if !ok {
			logger.LogErrorf("Type assertion error: casting subcommand: %v", sub)

			return
		}

		accountID, err := wallet.CreateAccount(ctx, params)
		if err != nil {
			logger.LogErrorf("Type assertion error: creating account: %v", err)

			return
		}

		logger.LogInfof("Created account %s", accountID)

	case accountwallet.OperationDestroyAccount:
		logger.LogInfof("Run subcommand: %s, with parametetr set: %v", accountwallet.OperationDestroyAccount, sub)
		params, ok := sub.(*accountwallet.DestroyAccountParams)
		if !ok {
			logger.LogErrorf("Type assertion error: casting subcommand: %v", sub)

			return
		}

		err := wallet.DestroyAccount(ctx, params)
		if err != nil {
			logger.LogErrorf("Error destroying account: %v", err)

			return
		}

	case accountwallet.OperationListAccounts:
		err := wallet.ListAccount()
		if err != nil {
			logger.LogErrorf("Error listing accounts: %v", err)

			return
		}

	case accountwallet.OperationAllotAccount:
		logger.LogInfof("Run subcommand: %s, with parametetr set: %v", accountwallet.OperationAllotAccount, sub)
		params, ok := sub.(*accountwallet.AllotAccountParams)
		if !ok {
			logger.LogErrorf("Type assertion error: casting subcommand: %v", sub)

			return
		}

		err := wallet.AllotToAccount(params)
		if err != nil {
			logger.LogErrorf("Error allotting account: %v", err)

			return
		}
	}
}
