package info

import (
	"context"
	"os"
	"strings"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/walletmanager"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
)

type Command string

func (c Command) String() string {
	return string(c)
}

const (
	CommandCommittee   Command = "committee"
	CommandValidators  Command = "validators"
	CommandAccounts    Command = "accounts"
	CommandDelegations Command = "delegations"
	CommandRewards     Command = "rewards"
)

func availableCommands(cmd string) bool {
	cmds := map[string]types.Empty{
		CommandCommittee.String():   types.Void,
		CommandValidators.String():  types.Void,
		CommandAccounts.String():    types.Void,
		CommandDelegations.String(): types.Void,
		CommandRewards.String():     types.Void,
	}

	_, ok := cmds[cmd]

	return ok
}

type ParametersInfo struct {
	Alias string `default:"" usage:"Alias for which info command should be executed."`
}

func Run(paramsTools *models.ParametersTool, paramsInfo *ParametersInfo, logger log.Logger) error {
	accManager, err := walletmanager.RunManager(logger,
		walletmanager.WithClientURL(paramsTools.NodeURLs[0]),
		walletmanager.WithAccountStatesFile(paramsTools.AccountStatesFile),
		walletmanager.WithFaucetAccountParams(walletmanager.NewGenesisAccountParams(paramsTools)),
		walletmanager.WithSilence(),
	)

	if err != nil {
		return err
	}

	manager := NewManager(logger, accManager)
	commands := parseInfoCommands(getCommands(os.Args[2:]))
	for _, cmd := range commands {
		err = infoSubcommand(context.Background(), manager, cmd, paramsInfo)
		if err != nil {
			return err
		}
	}

	return nil
}

//nolint:all,forcetypassert
func infoSubcommand(ctx context.Context, manager *Manager, cmd Command, params *ParametersInfo) error {
	switch cmd {
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
		if err := manager.DelegatorsInfo(); err != nil {
			return ierrors.Wrapf(err, "error while requesting delegations endpoint")
		}
	case CommandRewards:
		if err := manager.RewardsInfo(ctx, params); err != nil {
			return ierrors.Wrapf(err, "error gathering rewards info")
		}
	default:
		return ierrors.Errorf("unknown command: %s", cmd)
	}

	return nil
}

// getCommands gets the commands and ignores the parameters passed via command line.
func getCommands(args []string) []string {
	if len(args) == 0 {
		return nil
	}

	commands := make([]string, 0)
	for _, arg := range args {
		if strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "-") {
			continue
		}

		if !availableCommands(arg) {
			// skip as it might be a flag parameter
			continue
		}
		commands = append(commands, arg)
	}

	return commands
}

func parseInfoCommands(commands []string) []Command {
	parsedCmds := make([]Command, 0)

	for _, cmd := range commands {
		switch cmd {
		case CommandCommittee.String():
			parsedCmds = append(parsedCmds, CommandCommittee)

		case CommandValidators.String():
			parsedCmds = append(parsedCmds, CommandValidators)

		case CommandAccounts.String():
			parsedCmds = append(parsedCmds, CommandAccounts)

		case CommandDelegations.String():
			parsedCmds = append(parsedCmds, CommandDelegations)

		case CommandRewards.String():
			parsedCmds = append(parsedCmds, CommandRewards)

		default:
			return nil
		}
	}

	return parsedCmds
}

type Manager struct {
	accWallets *walletmanager.Manager
	log.Logger
}

func NewManager(logger log.Logger, accWallet *walletmanager.Manager) *Manager {
	return &Manager{
		accWallets: accWallet,
		Logger:     logger,
	}
}
