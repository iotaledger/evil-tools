package utils

import (
	"go.uber.org/zap"

	"github.com/iotaledger/hive.go/app/configuration"
	appLogger "github.com/iotaledger/hive.go/app/logger"
	"github.com/iotaledger/hive.go/logger"
)

var (
	NewLogger   = logger.NewLogger
	UtilsLogger *zap.SugaredLogger
)

func init() {
	config := configuration.New()
	err := config.Set(logger.ConfigurationKeyOutputPaths, []string{"evil-spammer.log", "stdout"})
	if err != nil {
		return
	}

	if err = appLogger.InitGlobalLogger(config); err != nil {
		panic(err)
	}
	logger.SetLevel(logger.LevelDebug)

	UtilsLogger = NewLogger("utils")
}
