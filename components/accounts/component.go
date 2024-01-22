package accounts

import (
	"context"
	"os"

	"go.uber.org/dig"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
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
		IsEnabled: func(_ *dig.Container) bool { return true },
	}
}

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In

	AccountWallets *accountwallet.AccountWallets
}

func run() error {
	Component.LogInfo("Starting evil-tools accounts ... done")
	// save wallet state on shutdown
	defer func() {
		err := accountwallet.SaveState(deps.AccountWallets)
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

func provideWallet() *accountwallet.AccountWallets {
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
		Component.LogPanic(err.Error())
	}

	return accWallet
}

func accountsSubcommands(ctx context.Context, wallets *accountwallet.AccountWallets, subcommands []accountwallet.AccountSubcommands) {
	for _, sub := range subcommands {
		err := accountsSubcommand(ctx, wallets, sub)
		if err != nil {
			Component.LogFatal(ierrors.Wrap(err, "failed to run subcommand").Error())

			return
		}
	}
}

//nolint:all,forcetypassert
func accountsSubcommand(ctx context.Context, wallets *accountwallet.AccountWallets, subCommand accountwallet.AccountSubcommands) error {
	Component.LogInfof("Run subcommand: %s, with parameter set: %v", subCommand.Type().String(), subCommand)

	switch subCommand.Type() {
	case accountwallet.OperationCreateAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*accountwallet.CreateAccountParams)

		accountID, err := wallets.CreateAccount(ctx, accParams)
		if err != nil {
			return ierrors.Wrap(err, "failed to create account")
		}

		Component.LogInfof("Created account %s", accountID)

	case accountwallet.OperationDestroyAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*accountwallet.DestroyAccountParams)

		if err := wallets.DestroyAccount(ctx, accParams); err != nil {
			return ierrors.Wrap(err, "failed to destroy account")
		}

	case accountwallet.OperationAllotAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*accountwallet.AllotAccountParams)

		if err := wallet.AllotToAccount(accParams); err != nil {
		if err := wallets.AllotToAccount(params); err != nil {
			return ierrors.Wrap(err, "failed to allot to account")
		}

	case accountwallet.OperationDelegateAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		params := subCommand.(*accountwallet.DelegateAccountParams)

		if err := wallets.DelegateToAccount(ctx, params); err != nil {
			return ierrors.Wrap(err, "failed to delegate to account")
		}

	case accountwallet.OperationRewards:
		//nolint:forcetypassert // we can safely assume that the type is correct
		params := subCommand.(*accountwallet.RewardsAccountParams)

		if err := wallets.Rewards(ctx, params); err != nil {
			return ierrors.Wrap(err, "failed to get rewards")
		}

	default:
		return ierrors.New("unknown subcommand")
	}

	return nil
}
