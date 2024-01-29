package spammer

import (
	"github.com/iotaledger/evil-tools/pkg/accountmanager"
	"github.com/iotaledger/evil-tools/programs"
	"github.com/iotaledger/hive.go/app"
)

const (
	ScriptName = "spammer"
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
	Component.LogInfo("Starting evil-tools spammer ... done")
	accWallet, err := accountmanager.RunManager(Component.Logger,
		accountmanager.WithClientURL(ParamsTool.NodeURLs[0]),
		accountmanager.WithFaucetURL(ParamsTool.FaucetURL),
		accountmanager.WithAccountStatesFile(ParamsTool.AccountStatesFile),
		accountmanager.WithFaucetAccountParams(&accountmanager.GenesisAccountParams{
			FaucetPrivateKey: ParamsTool.BlockIssuerPrivateKey,
			FaucetAccountID:  ParamsTool.AccountID,
		}),
	)
	if err != nil {
		Component.LogErrorf(err.Error())

		return err
	}

	programs.RunSpammer(
		Component.Daemon().ContextStopped(),
		Component.Logger,
		ParamsTool.NodeURLs,
		ParamsTool.FaucetURL,
		ParamsSpammer,
		accWallet)

	return nil

}
