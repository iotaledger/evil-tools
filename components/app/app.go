package app

import (
	"fmt"
	"os"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/app/components/profiling"
	"github.com/iotaledger/hive.go/app/components/shutdown"

	"github.com/iotaledger/evil-tools/components/eviltools"
)

var (
	// Name of the app.
	Name = "evil-spammer"

	// Version of the app.
	Version = "0.1.0"
)

func App() *app.App {
	return app.New(Name, Version,
		app.WithUsageText(fmt.Sprintf(`Usage of %s (%s %s):
Provide the first argument for the selected mode:
	'%s' - can be parametrized with additional flags to run one time spammer.
	'%s' - tool for account creation and transition.

Command line flags: %s`, os.Args[0], Name, Version, eviltools.ScriptSpammer, eviltools.ScriptAccounts, os.Args[0])),
		app.WithInitComponent(InitComponent),
		app.WithComponents(
			shutdown.Component,
			profiling.Component,
			eviltools.Component,
		),
	)
}

var InitComponent *app.InitComponent

func init() {
	InitComponent = &app.InitComponent{
		Component: &app.Component{
			Name: "App",
		},
		NonHiddenFlags: []string{
			"config",
			"help",
			"version",
		},
	}
}
