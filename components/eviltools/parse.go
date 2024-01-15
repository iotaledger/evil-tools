package eviltools

import (
	"fmt"
	"os"
	"strings"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/hive.go/ierrors"
)

const (
	AccountSubCommandCreate   = "create"
	AccountSubCommandConvert  = "convert"
	AccountSubCommandDestroy  = "destroy"
	AccountSubCommandAllot    = "allot"
	AccountSubCommandDelegate = "delegate"
	AccountSubCommandStake    = "stake"
	AccountSubCommandUpdate   = "update"
	AccountSubCommandList     = "list"
)

func getScript() (string, error) {
	if len(os.Args) <= 1 {
		return ScriptSpammer, nil
	}

	switch os.Args[1] {
	case ScriptSpammer, ScriptAccounts:
		return os.Args[1], nil
	default:
		return "", ierrors.Errorf("invalid script name: %s", os.Args[1])
	}
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

		if !accountwallet.AvailableCommands(arg) {
			// skip as it might be a flag parameter
			continue
		}
		commands = append(commands, arg)
	}

	return commands
}

func parseAccountCommands(commands []string, paramsAccounts *accountwallet.ParametersAccounts) []accountwallet.AccountSubcommands {
	parsedCmds := make([]accountwallet.AccountSubcommands, 0)

	for _, cmd := range commands {
		switch cmd {
		case AccountSubCommandCreate:
			createAccountParams, err := parseCreateAccountParams(&paramsAccounts.Create)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, createAccountParams)

		case AccountSubCommandConvert:
			convertAccountParams := parseConvertAccountFlags(&paramsAccounts.Convert)
			parsedCmds = append(parsedCmds, convertAccountParams)

		case AccountSubCommandDestroy:
			destroyAccountParams, err := parseDestroyAccountFlags(&paramsAccounts.Destroy)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, destroyAccountParams)

		case AccountSubCommandAllot:
			allotAccountParams, err := parseAllotAccountFlags(&paramsAccounts.Allot)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, allotAccountParams)

		case AccountSubCommandDelegate:
			delegatingAccountParams, err := parseDelegateAccountFlags(&paramsAccounts.Delegate)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, delegatingAccountParams)

		case AccountSubCommandStake:
			stakingAccountParams, err := parseStakeAccountFlags(&paramsAccounts.Stake)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, stakingAccountParams)

		case AccountSubCommandUpdate:
			updateAccountParams, err := parseUpdateAccountFlags(&paramsAccounts.Update)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, updateAccountParams)

		case AccountSubCommandList:
			parsedCmds = append(parsedCmds, &accountwallet.NoAccountParams{
				Operation: accountwallet.OperationListAccounts,
			})

		default:
			accountUsage()
			return nil
		}
	}

	return parsedCmds
}

func accountUsage() {
	fmt.Println("Usage for accounts [COMMAND] [FLAGS], multiple commands can be chained together.")
	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameCreateAccount)
	_, _ = parseCreateAccountParams(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameConvertAccount)
	_ = parseConvertAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameDestroyAccount)
	_, _ = parseDestroyAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameAllotAccount)
	_, _ = parseAllotAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameDelegateAccount)
	_, _ = parseDelegateAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.CmdNameStakeAccount)
	_, _ = parseStakeAccountFlags(nil)
}

func parseCreateAccountParams(paramsAccountCreate *accountwallet.ParametersAccountsCreate) (*accountwallet.CreateAccountParams, error) {
	if paramsAccountCreate == nil {
		return nil, ierrors.New("paramsAccountCreate missing for create account")
	}

	if paramsAccountCreate.Implicit && paramsAccountCreate.NoBlockIssuerFeature {
		return nil, ierrors.New("implicit account cannot be created without Block Issuance Feature")
	}

	if !paramsAccountCreate.Implicit && !paramsAccountCreate.Transition {
		Component.LogWarn("Implicit flag set to false, account will be created non-implicitly by Faucet, no need for transition, flag will be ignored")
		paramsAccountCreate.Transition = true
	}

	return &accountwallet.CreateAccountParams{
		Alias:      paramsAccountCreate.Alias,
		NoBIF:      paramsAccountCreate.NoBlockIssuerFeature,
		Implicit:   paramsAccountCreate.Implicit,
		Transition: paramsAccountCreate.Transition,
	}, nil
}

func parseConvertAccountFlags(paramsAccountConvert *accountwallet.ParametersAccountsConvert) *accountwallet.ConvertAccountParams {
	return &accountwallet.ConvertAccountParams{
		AccountAlias: paramsAccountConvert.Alias,
	}
}

func parseDestroyAccountFlags(paramsAccountDestroy *accountwallet.ParametersAccountsDestroy) (*accountwallet.DestroyAccountParams, error) {
	if paramsAccountDestroy == nil {
		return nil, ierrors.New("paramsAccountDestroy missing for destroy account")
	}

	return &accountwallet.DestroyAccountParams{
		AccountAlias: paramsAccountDestroy.Alias,
		ExpirySlot:   uint64(paramsAccountDestroy.ExpirySlot),
	}, nil
}

func parseAllotAccountFlags(paramsAccountAllot *accountwallet.ParametersAccountsAllot) (*accountwallet.AllotAccountParams, error) {
	if paramsAccountAllot == nil {
		return nil, ierrors.New("paramsAccountAllot missing for allot account")
	}

	return &accountwallet.AllotAccountParams{
		To:     paramsAccountAllot.AllotToAccount,
		Amount: uint64(paramsAccountAllot.Amount),
	}, nil
}

func parseStakeAccountFlags(paramsAccountStake *accountwallet.ParametersAccountsStake) (*accountwallet.StakeAccountParams, error) {
	if paramsAccountStake == nil {
		return nil, ierrors.New("paramsAccountStake missing for stake account")
	}

	return &accountwallet.StakeAccountParams{
		Alias:      paramsAccountStake.Alias,
		Amount:     uint64(paramsAccountStake.Amount),
		FixedCost:  uint64(paramsAccountStake.FixedCost),
		StartEpoch: uint64(paramsAccountStake.StartEpoch),
		EndEpoch:   uint64(paramsAccountStake.EndEpoch),
	}, nil
}

func parseDelegateAccountFlags(paramsAccountDelegate *accountwallet.ParametersAccountsDelegate) (*accountwallet.DelegateAccountParams, error) {
	if paramsAccountDelegate == nil {
		return nil, ierrors.New("paramsAccountDelegate missing for delegate account")
	}

	return &accountwallet.DelegateAccountParams{
		From:   paramsAccountDelegate.FromAccount,
		To:     paramsAccountDelegate.ToAccount,
		Amount: uint64(paramsAccountDelegate.Amount),
	}, nil
}

func parseUpdateAccountFlags(paramsAccountUpdate *accountwallet.ParametersAccountsUpdate) (*accountwallet.UpdateAccountParams, error) {
	if paramsAccountUpdate == nil {
		return nil, ierrors.New("paramsAccountUpdate missing for update account")
	}

	return &accountwallet.UpdateAccountParams{
		Alias:          paramsAccountUpdate.Alias,
		BlockIssuerKey: paramsAccountUpdate.BlockIssuerKey,
		Amount:         uint64(paramsAccountUpdate.Amount),
		Mana:           uint64(paramsAccountUpdate.Mana),
		ExpirySlot:     uint64(paramsAccountUpdate.ExpirySlot),
	}, nil
}
