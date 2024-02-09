package spammer

import (
	"context"

	"go.uber.org/dig"

	"github.com/iotaledger/evil-tools/pkg/evilwallet"
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/spammer"
	"github.com/iotaledger/evil-tools/pkg/walletmanager"
	"github.com/iotaledger/evil-tools/programs"
	"github.com/iotaledger/hive.go/app"
)

const (
	ScriptName = "spammer"
)

func init() {
	Component = &app.Component{
		Name:     "EvilTools",
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
	Component.LogInfo("Starting evil-tools spammer ... done")
	accManager, err := walletmanager.RunManager(Component.Logger,
		walletmanager.WithClientURL(deps.ParamsTool.NodeURLs[0]),
		walletmanager.WithFaucetURL(deps.ParamsTool.FaucetURL),
		walletmanager.WithAccountStatesFile(deps.ParamsTool.AccountStatesFile),
		walletmanager.WithFaucetAccountParams(walletmanager.NewGenesisAccountParams(deps.ParamsTool)),
	)
	if err != nil {
		Component.LogErrorf(err.Error())

		return err
	}

	evilWallet := evilwallet.NewEvilWallet(
		Component.Logger,
		evilwallet.WithClients(deps.ParamsTool.NodeURLs...),
		evilwallet.WithAccountsManager(accManager),
		evilwallet.WithFaucetClient(deps.ParamsTool.FaucetURL),
	)

	numOfInputs := spammer.EvaluateNumOfBatchInputs(ParamsSpammer)
	totalWalletsNeeded := spammer.BigWalletsNeeded(ParamsSpammer.Rate, ParamsSpammer.Duration, numOfInputs)
	minFaucetFundsDeposit := spammer.MinFaucetFundsDeposit(ParamsSpammer.Rate, ParamsSpammer.Duration, numOfInputs)

	err = Component.Daemon().BackgroundWorker("Funds Requesting", func(ctx context.Context) {
		programs.RequestFaucetFunds(ctx, Component.Logger, ParamsSpammer, evilWallet, totalWalletsNeeded, minFaucetFundsDeposit)
	})
	if err != nil {
		Component.Logger.LogError("error starting background worker for funds requesting ", err)
	}

	err = Component.Daemon().BackgroundWorker("Spammer", func(ctx context.Context) {
		programs.RunSpammer(ctx,
			Component.Logger,
			ParamsSpammer,
			evilWallet,
			minFaucetFundsDeposit)
	})
	if err != nil {
		Component.Logger.LogError("error starting background worker for funds requesting ", err)
	}

	Component.Daemon().Run()

	return nil

}
