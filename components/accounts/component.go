package accounts

import (
	"context"
	"os"

	"go.uber.org/dig"

	"github.com/iotaledger/evil-tools/pkg/accountmanager"
	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/ierrors"
)

const (
	ScriptName = "accounts"
)

func init() {
	Component = &app.Component{
		Name:     "Accounts",
		Params:   params,
		DepsFunc: func(cDeps dependencies) { deps = cDeps },
		Run:      run,
		Provide: func(c *dig.Container) error {
			return c.Provide(provideWallet)
		},
	}
}

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In

	AccountWallets *accountmanager.Manager
}

func run() error {
	Component.LogInfo("Starting evil-tools accounts ... done")
	// save wallet state on shutdown
	defer func() {
		err := deps.AccountWallets.SaveStateToFile()
		if err != nil {
			Component.LogErrorf("Error while saving wallet state: %v", err)
		}
	}()

	accountsSubcommandsFlags := parseAccountCommands(getCommands(os.Args[2:]), ParamsAccounts)
	accountsSubcommands(
		Component.Daemon().ContextStopped(),
		deps.AccountWallets,
		accountsSubcommandsFlags,
	)

	return nil
}

func provideWallet() *accountmanager.Manager {
	// load wallet
	accWallet, err := accountmanager.Run(Component.Daemon().ContextStopped(), Component.Logger,
		accountmanager.WithClientURL(ParamsAccounts.NodeURLs[0]),
		accountmanager.WithFaucetURL(ParamsAccounts.FaucetURL),
		accountmanager.WithAccountStatesFile(ParamsAccounts.AccountStatesFile),
		accountmanager.WithFaucetAccountParams(&accountmanager.GenesisAccountParams{
			FaucetPrivateKey: ParamsAccounts.BlockIssuerPrivateKey,
			FaucetAccountID:  ParamsAccounts.AccountID,
		}),
	)
	if err != nil {
		Component.LogPanic(err.Error())
	}

	return accWallet
}

func accountsSubcommands(ctx context.Context, wallets *accountmanager.Manager, subcommands []accountmanager.AccountSubcommands) {
	for _, sub := range subcommands {
		err := accountsSubcommand(ctx, wallets, sub)
		if err != nil {
			Component.LogFatal(ierrors.Wrap(err, "failed to run subcommand").Error())

			return
		}
	}
}

//nolint:all,forcetypassert
func accountsSubcommand(ctx context.Context, wallets *accountmanager.Manager, subCommand accountmanager.AccountSubcommands) error {
	Component.LogInfof("Run subcommand: %s, with parameter set: %v", subCommand.Type().String(), subCommand)

	switch subCommand.Type() {
	case accountmanager.OperationCreateAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*accountmanager.CreateAccountParams)

		accountID, err := wallets.CreateAccount(ctx, accParams)
		if err != nil {
			return ierrors.Wrap(err, "failed to create account")
		}

		Component.LogInfof("Created account %s", accountID)

	case accountmanager.OperationDestroyAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*accountmanager.DestroyAccountParams)

		if err := wallets.DestroyAccount(ctx, accParams); err != nil {
			return ierrors.Wrap(err, "failed to destroy account")
		}

	case accountmanager.OperationAllotAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*accountmanager.AllotAccountParams)

		if err := wallets.AllotToAccount(accParams); err != nil {
			return ierrors.Wrap(err, "failed to allot to account")
		}

	case accountmanager.OperationDelegateAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		params := subCommand.(*accountmanager.DelegateAccountParams)

		if err := wallets.DelegateToAccount(ctx, params); err != nil {
			return ierrors.Wrap(err, "failed to delegate to account")
		}

	case accountmanager.OperationRewardsAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		params := subCommand.(*accountmanager.RewardsAccountParams)

		if err := wallets.Rewards(ctx, params); err != nil {
			return ierrors.Wrap(err, "failed to get rewards")
		}

	default:
		return ierrors.New("unknown subcommand")
	}

	return nil
}
