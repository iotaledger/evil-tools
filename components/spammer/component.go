package spammer

import (
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

	programs.RunSpammer(
		Component.Daemon().ContextStopped(),
		Component.Logger,
		ParamsSpammer.NodeURLs,
		ParamsSpammer,
		nil) // todo provide AccountWallet

	return nil

}

//type dependencies struct {
//	dig.In
//
//	AccountWallet *accountwallet.AccountWallet
//}
