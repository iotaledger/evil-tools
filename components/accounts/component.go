package accounts

import (
	"context"
	"os"

	"go.uber.org/dig"

	"github.com/iotaledger/evil-tools/pkg/accountmanager"
	"github.com/iotaledger/evil-tools/pkg/models"
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

	accManager, err := accountmanager.RunManager(Component.Logger,
		accountmanager.WithClientURL(deps.ParamsTool.NodeURLs[0]),
		accountmanager.WithFaucetURL(deps.ParamsTool.FaucetURL),
		accountmanager.WithAccountStatesFile(deps.ParamsTool.AccountStatesFile),
		accountmanager.WithFaucetAccountParams(&accountmanager.GenesisAccountParams{
			FaucetPrivateKey: deps.ParamsTool.BlockIssuerPrivateKey,
			FaucetAccountID:  deps.ParamsTool.AccountID,
		}),
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

func accountsSubcommands(ctx context.Context, accManager *accountmanager.Manager, subcommands []accountmanager.AccountSubcommands) {
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

		if err := wallets.AllotToAccount(ctx, accParams); err != nil {
			return ierrors.Wrap(err, "failed to allot to account")
		}

	case accountmanager.OperationDelegate:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*accountmanager.DelegateAccountParams)

		if err := wallets.DelegateToAccount(ctx, accParams); err != nil {
			return ierrors.Wrap(err, "failed to delegate to account")
		}

	case accountmanager.OperationClaim:
		//nolint:forcetypassert // we can safely assume that the type is correct
		accParams := subCommand.(*accountmanager.ClaimAccountParams)

		if err := wallets.Claim(ctx, accParams); err != nil {
			return ierrors.Wrap(err, "failed to get rewards")
		}

	default:
		return ierrors.New("unknown subcommand")
	}

	return nil
}
