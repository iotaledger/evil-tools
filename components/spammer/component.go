package spammer

import (
	"go.uber.org/dig"

	"github.com/iotaledger/evil-tools/pkg/accountmanager"
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

var (
	Component *app.Component
	deps      dependencies
)

type dependencies struct {
	dig.In

	AccountManager *accountmanager.Manager
}

func run() error {
	Component.LogInfo("Starting evil-tools spammer ... done")

	programs.RunSpammer(
		Component.Daemon().ContextStopped(),
		Component.Logger,
		ParamsSpammer.NodeURLs,
		ParamsSpammer,
		deps.AccountManager)

	return nil

}
