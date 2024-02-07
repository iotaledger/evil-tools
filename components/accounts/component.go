package accounts

import (
	"context"
	"os"

	"go.uber.org/dig"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/walletmanager"
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
		Run:      run,
		DepsFunc: func(cDeps dependencies) { deps = cDeps },
	}
}

type dependencies struct {
	dig.In

	ParamsTool *models.ParametersTool
}

var (
	Component *app.Component
	deps      dependencies
)

func run() error {
	Component.LogInfo("Starting evil-tools accounts ... done")

	accManager, err := walletmanager.RunManager(Component.Logger,
		walletmanager.WithClientURL(deps.ParamsTool.NodeURLs[0]),
		walletmanager.WithFaucetURL(deps.ParamsTool.FaucetURL),
		walletmanager.WithAccountStatesFile(deps.ParamsTool.AccountStatesFile),
		walletmanager.WithFaucetAccountParams(walletmanager.NewGenesisAccountParams(deps.ParamsTool)),
	)
	if err != nil {
		Component.LogError(err.Error())

		return err
	}

	accountsSubcommandsFlags := parseAccountCommands(getCommands(os.Args[2:]), ParamsAccounts)
	accountsSubcommands(
		Component.Daemon().ContextStopped(),
		accManager,
		accountsSubcommandsFlags,
	)

	return nil
}

func accountsSubcommands(ctx context.Context, accManager *walletmanager.Manager, subcommands []walletmanager.AccountSubcommands) {
	// save wallet state on shutdown
	defer func() {
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
func accountsSubcommand(ctx context.Context, wallets *walletmanager.Manager, subCommand walletmanager.AccountSubcommands) error {
	Component.LogInfof("Run subcommand: %s, with parameter set: %v", subCommand.Type().String(), subCommand)

	switch subCommand.Type() {
	case walletmanager.OperationCreateAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*walletmanager.CreateAccountParams)

		accountID, err := wallets.CreateAccount(ctx, accParams)
		if err != nil {
			return ierrors.Wrap(err, "failed to create account")
		}

		Component.LogInfof("Created account %s", accountID)

	case walletmanager.OperationDestroyAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*walletmanager.DestroyAccountParams)

		if err := wallets.DestroyAccount(ctx, accParams); err != nil {
			return ierrors.Wrap(err, "failed to destroy account")
		}

	case walletmanager.OperationAllotAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*walletmanager.AllotAccountParams)

		if err := wallets.AllotToAccount(ctx, accParams); err != nil {
			return ierrors.Wrap(err, "failed to allot to account")
		}

	case walletmanager.OperationDelegate:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*walletmanager.DelegateAccountParams)

		if err := wallets.DelegateToAccount(ctx, accParams); err != nil {
			return ierrors.Wrap(err, "failed to delegate to account")
		}

	case walletmanager.OperationClaim:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*walletmanager.ClaimAccountParams)

		if err := wallets.Claim(ctx, accParams); err != nil {
			return ierrors.Wrap(err, "failed to get rewards")
		}

	default:
		return ierrors.New("unknown subcommand")
	}

	return nil
}
