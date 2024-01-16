package accounts

import (
	"context"
	"os"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/ierrors"
)

const (
	ScriptName = "accounts"
)

func init() {
	Component = &app.Component{
		Name:   "Accounts",
		Params: params,
		Run:    run,
	}
}

var (
	Component *app.Component
)

func run() error {
	Component.LogInfo("Starting evil-tools accounts ... done")

	var accWallet *accountwallet.AccountWallet
	// load wallet
	accWallet, err := accountwallet.Run(Component.Logger,
		accountwallet.WithClientURL(ParamsAccounts.NodeURLs[0]),
		accountwallet.WithFaucetURL(ParamsAccounts.FaucetURL),
		accountwallet.WithAccountStatesFile(ParamsAccounts.AccountStatesFile),
		accountwallet.WithFaucetAccountParams(&accountwallet.GenesisAccountParams{
			FaucetPrivateKey: ParamsAccounts.BlockIssuerPrivateKey,
			FaucetAccountID:  ParamsAccounts.AccountID,
		}),
	)
	if err != nil {
		Component.LogFatal(ierrors.Wrap(err, "failed to init account wallet").Error())
	}

	// save wallet state on shutdown
	defer func() {
		err = accountwallet.SaveState(accWallet)
		if err != nil {
			Component.LogErrorf("Error while saving wallet state: %v", err)
		}
	}()

	accountsSubcommandsFlags := parseAccountCommands(getCommands(os.Args[2:]), ParamsAccounts)
	accountsSubcommands(
		Component.Daemon().ContextStopped(),
		accWallet,
		accountsSubcommandsFlags,
	)

	return nil
}

// TODO provide account wallet
//func provide(c *dig.Container) error {
//	if err := c.Provide(func() *accountwallet.AccountWallet {
//		return nil
//	}); err != nil {
//		Component.LogPanic(err.Error())
//	}
//
//	return nil
//}

func accountsSubcommands(ctx context.Context, wallet *accountwallet.AccountWallet, subcommands []accountwallet.AccountSubcommands) {
	for _, sub := range subcommands {
		err := accountsSubcommand(ctx, wallet, sub)
		if err != nil {
			Component.LogFatal(ierrors.Wrap(err, "failed to run subcommand").Error())

			return
		}
	}
}

//nolint:all,forcetypassert
func accountsSubcommand(ctx context.Context, wallet *accountwallet.AccountWallet, subCommand accountwallet.AccountSubcommands) error {
	Component.LogInfof("Run subcommand: %s, with parameter set: %v", subCommand.Type().String(), subCommand)

	switch subCommand.Type() {
	case accountwallet.OperationCreateAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		params := subCommand.(*accountwallet.CreateAccountParams)

		accountID, err := wallet.CreateAccount(ctx, params)
		if err != nil {
			return ierrors.Wrap(err, "failed to create account")
		}

		Component.LogInfof("Created account %s", accountID)

	case accountwallet.OperationDestroyAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		params := subCommand.(*accountwallet.DestroyAccountParams)

		if err := wallet.DestroyAccount(ctx, params); err != nil {
			return ierrors.Wrap(err, "failed to destroy account")
		}

	case accountwallet.OperationListAccounts:
		if err := wallet.ListAccount(); err != nil {
			return ierrors.Wrap(err, "failed to list accounts")
		}

	case accountwallet.OperationAllotAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		params := subCommand.(*accountwallet.AllotAccountParams)

		if err := wallet.AllotToAccount(params); err != nil {
			return ierrors.Wrap(err, "failed to allot to account")
		}
	default:
		return ierrors.New("unknown subcommand")
	}

	return nil
}
