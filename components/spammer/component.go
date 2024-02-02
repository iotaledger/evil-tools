package spammer

import (
	"go.uber.org/dig"

	"github.com/iotaledger/evil-tools/pkg/models"
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
	accWallet, err := walletmanager.RunManager(Component.Logger,
		walletmanager.WithClientURL(deps.ParamsTool.NodeURLs[0]),
		walletmanager.WithFaucetURL(deps.ParamsTool.FaucetURL),
		walletmanager.WithAccountStatesFile(deps.ParamsTool.AccountStatesFile),
		walletmanager.WithFaucetAccountParams(&walletmanager.GenesisAccountParams{
			FaucetPrivateKey: deps.ParamsTool.BlockIssuerPrivateKey,
			FaucetAccountID:  deps.ParamsTool.AccountID,
		}),
	)
	if err != nil {
		Component.LogErrorf(err.Error())

		return err
	}

	programs.RunSpammer(
		Component.Daemon().ContextStopped(),
		Component.Logger,
		deps.ParamsTool.NodeURLs,
		deps.ParamsTool.FaucetURL,
		ParamsSpammer,
		accWallet)

	return nil

}
