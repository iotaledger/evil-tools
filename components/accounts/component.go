package accounts

import (
	"context"
	"fmt"
	"os"

	"github.com/iotaledger/evil-tools/pkg/accountmanager"
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

	accManager, err := accountmanager.RunManager(Component.Logger,
		accountmanager.WithClientURL(ParamsTool.NodeURLs[0]),
		accountmanager.WithFaucetURL(ParamsTool.FaucetURL),
		accountmanager.WithAccountStatesFile(ParamsTool.AccountStatesFile),
		accountmanager.WithFaucetAccountParams(&accountmanager.GenesisAccountParams{
			FaucetPrivateKey: ParamsTool.BlockIssuerPrivateKey,
			FaucetAccountID:  ParamsTool.AccountID,
		}),
	)
	if err != nil {
		Component.LogPanic(err.Error())
	}

	accountsSubcommandsFlags := parseAccountCommands(getCommands(os.Args[2:]), ParamsAccounts)
	accountsSubcommands(
		Component.Daemon().ContextStopped(),
		accManager,
		accountsSubcommandsFlags,
	)

	return nil
}

func accountsSubcommands(ctx context.Context, accManager *accountmanager.Manager, subcommands []accountmanager.AccountSubcommands) {
	// save wallet state on shutdown
	defer func() {
		fmt.Println("Saving wallet state...")
		err := accManager.SaveStateToFile()
		if err != nil {
			Component.LogErrorf("Error while saving wallet state: %v", err)
		}
	}()

	for _, sub := range subcommands {
		err := accountsSubcommand(ctx, accManager, sub)
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
		accParams := subCommand.(*accountmanager.DelegateAccountParams)

		if err := wallets.DelegateToAccount(ctx, accParams); err != nil {
			return ierrors.Wrap(err, "failed to delegate to account")
		}

	case accountmanager.OperationRewardsAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*accountmanager.RewardsAccountParams)

		if err := wallets.Rewards(ctx, accParams); err != nil {
			return ierrors.Wrap(err, "failed to get rewards")
		}

	default:
		return ierrors.New("unknown subcommand")
	}

	return nil
}
