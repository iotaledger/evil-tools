package info

import (
	"strings"

	"github.com/iotaledger/hive.go/ds/types"
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
