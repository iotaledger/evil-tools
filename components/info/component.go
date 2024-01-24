package info

import (
	"os"

	"github.com/iotaledger/evil-tools/pkg/info"
	"github.com/iotaledger/hive.go/app"
)

const ScriptName = "info"

// no need for run function as we use Component only to load parameters

func init() {
	Component = &app.Component{
		Name:   "Info",
		Params: params,
	}
}

var (
	Component *app.Component
)

func Run() error {
	loggerConfig := &app.LoggerConfig{
		Name:        "info",
		Level:       "info",
		TimeFormat:  "rfc3339",
		OutputPaths: []string{"stdout"},
	}
	logger, err := app.NewLoggerFromConfig(loggerConfig)
	if err != nil {
		return err
	}
	err = info.Run(ParamsTool, logger)
	if err != nil {
		return err
	}

	os.Exit(0)

	return nil
}
