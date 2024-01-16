package info

import (
	"github.com/iotaledger/hive.go/app"
)

const ScriptName = "info"

func init() {
	Component = &app.Component{
		Name:   "Info",
		Params: params,
		Run:    run,
	}
}

var (
	Component *app.Component
)

func run() error {
	Component.LogInfo("Start info component ... done")

	return nil
}
