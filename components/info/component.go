package info

import (
	"context"
	"os"

	"go.uber.org/dig"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/evil-tools/pkg/info"
	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/ierrors"
)

const ScriptName = "info"

func init() {
	Component = &app.Component{
		Name:     "Info",
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

	AccountWallets *accountwallet.AccountWallets
}

func run() error {
	Component.LogInfo("Start info component ... done")
	manager := info.NewManager(Component.Logger, deps.AccountWallets)

	commands := parseInfoCommands(getCommands(os.Args[2:]))

	for _, cmd := range commands {
		err := infoSubcommand(context.Background(), manager, cmd)
		if err != nil {
			Component.LogFatal(err.Error())
		}
	}

	return nil
}

//nolint:all,forcetypassert
func infoSubcommand(ctx context.Context, manager *info.Manager, subCommand Command) error {
	switch subCommand {
	case CommandCommittee:
		if err := manager.CommitteeInfo(ctx); err != nil {
			return ierrors.Wrapf(err, "error while requesting committee endpoint")
		}
	case CommandValidators:
		if err := manager.ValidatorsInfo(ctx); err != nil {
			return ierrors.Wrapf(err, "error while requesting stakers endpoint")
		}
	case CommandAccounts:
		if err := manager.AccountsInfo(); err != nil {
			return ierrors.Wrapf(err, "error while requesting accounts endpoint")
		}
	case CommandDelegations:
		if err := manager.DelegatorsInfo(ctx); err != nil {
			return ierrors.Wrapf(err, "error while requesting delegations endpoint")
		}
	default:
		return ierrors.Errorf("unknown command: %s", subCommand)
	}

	return nil
}
