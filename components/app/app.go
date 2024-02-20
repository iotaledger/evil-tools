package app

import (
	"fmt"
	"os"

	"go.uber.org/dig"

	"github.com/iotaledger/evil-tools/components/accounts"
	"github.com/iotaledger/evil-tools/components/info"
	"github.com/iotaledger/evil-tools/components/shutdown"
	"github.com/iotaledger/evil-tools/components/spammer"
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/app/components/profiling"
	"github.com/iotaledger/hive.go/ierrors"
)

var (
	// Name of the app.
	Name = "evil-spammer"

	// Version of the app.
	Version = "v0.1.0-develop"
)

func App() *app.App {
	components := []*app.Component{shutdown.Component, profiling.Component}
	script, err := getScript()
	if err != nil {
		panic(err)
	}
	switch script {
	case spammer.ScriptName:
		components = append(components, spammer.Component)
	case accounts.ScriptName:
		components = append(components, accounts.Component)
	case info.ScriptName:
		components = append(components, info.Component)
	default:
		// used to generate configuration.md file, all components need to run to be listed
		components = append(components, info.Component)
		components = append(components, accounts.Component)
		components = append(components, spammer.Component)
	}

	return app.New(Name, Version,
		app.WithUsageText(fmt.Sprintf(`Usage of %s (%s %s):
Provide the first argument for the selected mode:
	'%s' - can be parametrized with additional flags to run one time spammer.
	'%s' - tool for account creation and transition.
	'%s' - listing details about stored accounts and node.

Command line flags: %s`, os.Args[0], Name, Version, spammer.ScriptName, accounts.ScriptName, info.ScriptName, os.Args[0])),
		app.WithInitComponent(InitComponent),
		app.WithComponents(components...),
	)
}

var InitComponent *app.InitComponent

func init() {
	InitComponent = &app.InitComponent{
		Component: &app.Component{
			Name:   "App",
			Params: params,
			Provide: func(c *dig.Container) error {
				return c.Provide(func() *models.ParametersTool {
					return models.ParamsTool
				})
			},
		},
		Init: initialize,
		NonHiddenFlags: []string{
			"config",
			"help",
			"version",
		},
	}
}

func getScript() (string, error) {
	if len(os.Args) <= 1 {
		return "", nil
	}

	switch os.Args[1] {
	case spammer.ScriptName, accounts.ScriptName, info.ScriptName:
		return os.Args[1], nil
	default:
		return "", ierrors.Errorf("invalid script name: %s", os.Args[1])
	}
}

func initialize(_ *app.App) error {
	if len(os.Args) > 1 && info.ScriptName == os.Args[1] {
		err := info.Run()
		if err != nil {
			return err
		}
	}

	return nil
}
