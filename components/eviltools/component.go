package eviltools

import (
	"context"
	"os"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/evil-tools/programs"
	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/ierrors"
)

const (
	ScriptSpammer  = "spammer"
	ScriptAccounts = "accounts"
)

func init() {
	Component = &app.Component{
		Name:   "EvilTools",
		Params: params,
		Run:    run,
	}
}

var (
	Component *app.Component
)

func run() error {
	Component.LogInfo("Starting evil-tools ... done")

	script, err := getScript()
	if err != nil {
		Component.LogFatal(err.Error())
	}

	Component.LogInfof("script %s", script)

	var accWallet *accountwallet.AccountWallet
	if script == ScriptSpammer || script == ScriptAccounts {
		// load wallet
		accWallet, err = accountwallet.Run(Component.Logger,
			accountwallet.WithClientURL(ParamsEvilTools.NodeURLs[0]),
			accountwallet.WithFaucetURL(ParamsEvilTools.FaucetURL),
			accountwallet.WithAccountStatesFile(ParamsEvilTools.Accounts.AccountStatesFile),
			accountwallet.WithFaucetAccountParams(&accountwallet.GenesisAccountParams{
				FaucetPrivateKey: ParamsEvilTools.Accounts.BlockIssuerPrivateKey,
				FaucetAccountID:  ParamsEvilTools.Accounts.AccountID,
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
	}

	// run selected test scenario
	switch script {
	case ScriptSpammer:
		programs.RunSpammer(
			Component.Daemon().ContextStopped(),
			Component.Logger,
			ParamsEvilTools.NodeURLs,
			&ParamsEvilTools.Spammer,
			accWallet)
	case ScriptAccounts:
		accountsSubcommandsFlags := parseAccountCommands(getCommands(os.Args[2:]), &ParamsEvilTools.Accounts)
		accountsSubcommands(
			Component.Daemon().ContextStopped(),
			accWallet,
			accountsSubcommandsFlags,
		)
	}

	return nil
}

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

	case accountwallet.OperationDelegateAccount:
		//nolint:forcetypassert // we can safely assume that the type is correct
		params := subCommand.(*accountwallet.DelegateAccountParams)

		if err := wallet.DelegateToAccount(ctx, params); err != nil {
			return ierrors.Wrap(err, "failed to delegate to account")
		}
	case accountwallet.OperationRewards:
		//nolint:forcetypassert // we can safely assume that the type is correct
		params := subCommand.(*accountwallet.RewardsAccountParams)

		if err := wallet.Rewards(ctx, params); err != nil {
			return ierrors.Wrap(err, "failed to get rewards")
		}
	default:
		return ierrors.New("unknown subcommand")
	}

	return nil
}
