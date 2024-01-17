package accounts

import (
	"fmt"
	"strings"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/hive.go/ierrors"
)

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

func parseAccountCommands(commands []string, paramsAccounts *ParametersAccounts) []accountwallet.AccountSubcommands {
	parsedCmds := make([]accountwallet.AccountSubcommands, 0)

	for _, cmd := range commands {
		switch cmd {
		case accountwallet.OperationCreateAccount.String():
			createAccountParams, err := parseCreateAccountParams(&paramsAccounts.Create)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, createAccountParams)

		case accountwallet.OperationConvertAccount.String():
			convertAccountParams := parseConvertAccountFlags(&paramsAccounts.Convert)
			parsedCmds = append(parsedCmds, convertAccountParams)

		case accountwallet.OperationDestroyAccount.String():
			destroyAccountParams, err := parseDestroyAccountFlags(&paramsAccounts.Destroy)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, destroyAccountParams)

		case accountwallet.OperationAllotAccount.String():
			allotAccountParams, err := parseAllotAccountFlags(&paramsAccounts.Allot)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, allotAccountParams)

		case accountwallet.OperationDelegateAccount.String():
			delegatingAccountParams, err := parseDelegateAccountFlags(&paramsAccounts.Delegate)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, delegatingAccountParams)

		case accountwallet.OperationStakeAccount.String():
			stakingAccountParams, err := parseStakeAccountFlags(&paramsAccounts.Stake)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, stakingAccountParams)

		case accountwallet.OperationUpdateAccount.String():
			updateAccountParams, err := parseUpdateAccountFlags(&paramsAccounts.Update)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, updateAccountParams)

		default:
			accountUsage()
			return nil
		}
	}

	return parsedCmds
}

func accountUsage() {
	fmt.Println("Usage for accounts [COMMAND] [FLAGS], multiple commands can be chained together.")
	fmt.Printf("COMMAND: %s\n", accountwallet.OperationCreateAccount)
	_, _ = parseCreateAccountParams(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.OperationConvertAccount)
	_ = parseConvertAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.OperationDestroyAccount)
	_, _ = parseDestroyAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.OperationAllotAccount)
	_, _ = parseAllotAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.OperationDelegateAccount)
	_, _ = parseDelegateAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountwallet.OperationStakeAccount)
	_, _ = parseStakeAccountFlags(nil)
}

func parseCreateAccountParams(paramsAccountCreate *ParametersAccountsCreate) (*accountwallet.CreateAccountParams, error) {
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

func parseConvertAccountFlags(paramsAccountConvert *ParametersAccountsConvert) *accountwallet.ConvertAccountParams {
	return &accountwallet.ConvertAccountParams{
		AccountAlias: paramsAccountConvert.Alias,
	}
}

func parseDestroyAccountFlags(paramsAccountDestroy *ParametersAccountsDestroy) (*accountwallet.DestroyAccountParams, error) {
	if paramsAccountDestroy == nil {
		return nil, ierrors.New("paramsAccountDestroy missing for destroy account")
	}

	return &accountwallet.DestroyAccountParams{
		AccountAlias: paramsAccountDestroy.Alias,
		ExpirySlot:   uint64(paramsAccountDestroy.ExpirySlot),
	}, nil
}

func parseAllotAccountFlags(paramsAccountAllot *ParametersAccountsAllot) (*accountwallet.AllotAccountParams, error) {
	if paramsAccountAllot == nil {
		return nil, ierrors.New("paramsAccountAllot missing for allot account")
	}

	return &accountwallet.AllotAccountParams{
		To:     paramsAccountAllot.AllotToAccount,
		Amount: uint64(paramsAccountAllot.Amount),
	}, nil
}

func parseStakeAccountFlags(paramsAccountStake *ParametersAccountsStake) (*accountwallet.StakeAccountParams, error) {
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

func parseDelegateAccountFlags(paramsAccountDelegate *ParametersAccountsDelegate) (*accountwallet.DelegateAccountParams, error) {
	if paramsAccountDelegate == nil {
		return nil, ierrors.New("paramsAccountDelegate missing for delegate account")
	}

	return &accountwallet.DelegateAccountParams{
		From:   paramsAccountDelegate.FromAccount,
		To:     paramsAccountDelegate.ToAccount,
		Amount: uint64(paramsAccountDelegate.Amount),
	}, nil
}

func parseUpdateAccountFlags(paramsAccountUpdate *ParametersAccountsUpdate) (*accountwallet.UpdateAccountParams, error) {
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
