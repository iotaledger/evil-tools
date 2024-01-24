package info

import (
	"context"
	"os"
	"strings"

	"github.com/iotaledger/evil-tools/pkg/accountmanager"
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
)

type Command string

func (a Command) String() string {
	return string(a)
}

const (
	CommandCommittee   Command = "committee"
	CommandValidators  Command = "validators"
	CommandAccounts    Command = "accounts"
	CommandDelegations Command = "delegations"
)

func availableCommands(cmd string) bool {
	cmds := map[string]types.Empty{
		CommandCommittee.String():   types.Void,
		CommandValidators.String():  types.Void,
		CommandAccounts.String():    types.Void,
		CommandDelegations.String(): types.Void,
	}

	_, ok := cmds[cmd]

	return ok
}

func Run(params *models.ParametersTool, logger log.Logger) error {
	accManager, err := accountmanager.RunManager(logger,
		accountmanager.WithClientURL(params.NodeURLs[0]),
		accountmanager.WithAccountStatesFile(params.AccountStatesFile),
		accountmanager.WithFaucetAccountParams(&accountmanager.GenesisAccountParams{
			FaucetPrivateKey: params.BlockIssuerPrivateKey,
			FaucetAccountID:  params.AccountID,
		}),
		accountmanager.WithSilence(),
	)

	if err != nil {
		return err
	}

	manager := NewManager(logger, accManager)
	commands := parseInfoCommands(getCommands(os.Args[2:]))
	for _, cmd := range commands {
		err = infoSubcommand(context.Background(), manager, cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

//nolint:all,forcetypassert
func infoSubcommand(ctx context.Context, manager *Manager, subCommand Command) error {
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
		if err := manager.DelegatorsInfo(); err != nil {
			return ierrors.Wrapf(err, "error while requesting delegations endpoint")
		}
	default:
		return ierrors.Errorf("unknown command: %s", subCommand)
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

		default:
			return nil
		}
	}

	return parsedCmds
}

type Manager struct {
	accWallets *accountmanager.Manager
	logger     log.Logger
}

func NewManager(logger log.Logger, accWallet *accountmanager.Manager) *Manager {
	return &Manager{
		accWallets: accWallet,
		logger:     logger,
	}
}
