package main

import (
	"flag"
	"fmt"

	"github.com/iotaledger/evil-tools/accountwallet"
	"github.com/iotaledger/evil-tools/interactive"
	"github.com/iotaledger/evil-tools/logger"
	"github.com/iotaledger/evil-tools/programs"
)

var (
	log           = logger.New("main")
	optionFlagSet = flag.NewFlagSet("script flag set", flag.ExitOnError)
)

const (
	ScriptInteractive = "interactive"
	ScriptSpammer     = "spammer"
	ScriptAccounts    = "accounts"
)

func main() {
	help := parseFlags()

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
	if Script == ScriptSpammer || Script == ScriptAccounts {
		// read config here
		config := accountwallet.LoadConfiguration()
		// load wallet
		accWallet, err = accountwallet.Run(config)
		if err != nil {
			log.Error(err)
			log.Errorf("Failed to init account wallet, exitting...")

			return
		}

		// save wallet and latest faucet output
		defer func() {
			err = accountwallet.SaveState(accWallet)
			if err != nil {
				log.Errorf("Error while saving wallet state: %v", err)
			}
			accountwallet.SaveConfiguration(config)

		}()
	}
	// run selected test scenario
	switch Script {
	case ScriptInteractive:
		interactive.Run()
	case ScriptSpammer:
		programs.CustomSpam(&customSpamParams, accWallet)
	case ScriptAccounts:
		accountsSubcommands(accWallet, accountsSubcommandsFlags)
	default:
		log.Warnf("Unknown parameter for script, possible values: interactive, spammer, accounts")
	}
}

func accountsSubcommands(wallet *accountwallet.AccountWallet, subcommands []accountwallet.AccountSubcommands) {
	for _, sub := range subcommands {
		accountsSubcommand(wallet, sub)
	}
}

//nolint:all,forcetypassert
func accountsSubcommand(wallet *accountwallet.AccountWallet, sub accountwallet.AccountSubcommands) {
	switch sub.Type() {
	case accountwallet.OperationCreateAccount:
		log.Infof("Run subcommand: %s, with parametetr set: %v", accountwallet.OperationCreateAccount.String(), sub)
		params, ok := sub.(*accountwallet.CreateAccountParams)
		if !ok {
			log.Errorf("Type assertion error: casting subcommand: %v", sub)

			return
		}

		accountID, err := wallet.CreateAccount(params)
		if err != nil {
			log.Errorf("Type assertion error: creating account: %v", err)

			return
		}

		log.Infof("Created account %s", accountID)

	case accountwallet.OperationDestroyAccount:
		log.Infof("Run subcommand: %s, with parametetr set: %v", accountwallet.OperationDestroyAccount, sub)
		params, ok := sub.(*accountwallet.DestroyAccountParams)
		if !ok {
			log.Errorf("Type assertion error: casting subcommand: %v", sub)

			return
		}

		err := wallet.DestroyAccount(params)
		if err != nil {
			log.Errorf("Error destroying account: %v", err)

			return
		}

	case accountwallet.OperationListAccounts:
		err := wallet.ListAccount()
		if err != nil {
			log.Errorf("Error listing accounts: %v", err)

			return
		}

	case accountwallet.OperationAllotAccount:
		log.Infof("Run subcommand: %s, with parametetr set: %v", accountwallet.OperationAllotAccount, sub)
		params, ok := sub.(*accountwallet.AllotAccountParams)
		if !ok {
			log.Errorf("Type assertion error: casting subcommand: %v", sub)

			return
		}

		err := wallet.AllotToAccount(params)
		if err != nil {
			log.Errorf("Error allotting account: %v", err)

			return
		}
	}
}
