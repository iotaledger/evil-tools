package accounts

import (
	"fmt"
	"strings"

	"github.com/iotaledger/evil-tools/pkg/accountmanager"
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
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

		if !accountmanager.AvailableCommands(arg) {
			// skip as it might be a flag parameter
			continue
		}
		commands = append(commands, arg)
	}

	return commands
}

func parseAccountCommands(commands []string, paramsAccounts *ParametersAccounts) []accountmanager.AccountSubcommands {
	parsedCmds := make([]accountmanager.AccountSubcommands, 0)

	for _, cmd := range commands {
		switch cmd {
		case accountmanager.OperationCreateAccount.String():
			createAccountParams, err := parseCreateAccountParams(&paramsAccounts.Create)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, createAccountParams)

		case accountmanager.OperationConvertAccount.String():
			convertAccountParams := parseConvertAccountFlags(&paramsAccounts.Convert)
			parsedCmds = append(parsedCmds, convertAccountParams)

		case accountmanager.OperationDestroyAccount.String():
			destroyAccountParams, err := parseDestroyAccountFlags(&paramsAccounts.Destroy)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, destroyAccountParams)

		case accountmanager.OperationAllotAccount.String():
			allotAccountParams, err := parseAllotAccountFlags(&paramsAccounts.Allot)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, allotAccountParams)

		case accountmanager.OperationDelegate.String():
			delegatingAccountParams, err := parseDelegateAccountFlags(&paramsAccounts.Delegate)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, delegatingAccountParams)

		case accountmanager.OperationStakeAccount.String():
			stakingAccountParams, err := parseStakeAccountFlags(&paramsAccounts.Stake)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, stakingAccountParams)

		case accountmanager.OperationClaim.String():
			rewardsParams, err := parseRewardsFlags(&paramsAccounts.Rewards)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, rewardsParams)

		case accountmanager.OperationUpdateAccount.String():
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
	fmt.Printf("COMMAND: %s\n", accountmanager.OperationCreateAccount)
	_, _ = parseCreateAccountParams(nil)

	fmt.Printf("COMMAND: %s\n", accountmanager.OperationConvertAccount)
	_ = parseConvertAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountmanager.OperationDestroyAccount)
	_, _ = parseDestroyAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountmanager.OperationAllotAccount)
	_, _ = parseAllotAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountmanager.OperationDelegate)
	_, _ = parseDelegateAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountmanager.OperationStakeAccount)
	_, _ = parseStakeAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", accountmanager.OperationClaim)
	_, _ = parseRewardsFlags(nil)
}

func parseCreateAccountParams(paramsAccountCreate *ParametersAccountsCreate) (*accountmanager.CreateAccountParams, error) {
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

	return &accountmanager.CreateAccountParams{
		Alias:      paramsAccountCreate.Alias,
		NoBIF:      paramsAccountCreate.NoBlockIssuerFeature,
		Implicit:   paramsAccountCreate.Implicit,
		Transition: paramsAccountCreate.Transition,
	}, nil
}

func parseConvertAccountFlags(paramsAccountConvert *ParametersAccountsConvert) *accountmanager.ConvertAccountParams {
	return &accountmanager.ConvertAccountParams{
		AccountAlias: paramsAccountConvert.Alias,
	}
}

func parseDestroyAccountFlags(paramsAccountDestroy *ParametersAccountsDestroy) (*accountmanager.DestroyAccountParams, error) {
	if paramsAccountDestroy == nil {
		return nil, ierrors.New("paramsAccountDestroy missing for destroy account")
	}

	return &accountmanager.DestroyAccountParams{
		AccountAlias: paramsAccountDestroy.Alias,
		ExpirySlot:   uint64(paramsAccountDestroy.ExpirySlot),
	}, nil
}

func parseAllotAccountFlags(paramsAccountAllot *ParametersAccountsAllot) (*accountmanager.AllotAccountParams, error) {
	if paramsAccountAllot == nil {
		return nil, ierrors.New("paramsAccountAllot missing for allot account")
	}

	return &accountmanager.AllotAccountParams{
		Alias:  paramsAccountAllot.Alias,
		Amount: paramsAccountAllot.Amount,
	}, nil
}

func parseStakeAccountFlags(paramsAccountStake *ParametersAccountsStake) (*accountmanager.StakeAccountParams, error) {
	if paramsAccountStake == nil {
		return nil, ierrors.New("paramsAccountStake missing for stake account")
	}

	return &accountmanager.StakeAccountParams{
		Alias:      paramsAccountStake.Alias,
		Amount:     uint64(paramsAccountStake.Amount),
		FixedCost:  uint64(paramsAccountStake.FixedCost),
		StartEpoch: uint64(paramsAccountStake.StartEpoch),
		EndEpoch:   uint64(paramsAccountStake.EndEpoch),
	}, nil
}

func parseRewardsFlags(paramsRewards *ParametersRewards) (*accountmanager.ClaimAccountParams, error) {
	if paramsRewards == nil {
		return nil, ierrors.New("paramsRewards missing for rewards account")
	}

	return &accountmanager.ClaimAccountParams{
		Alias: paramsRewards.Alias,
	}, nil
}

func parseDelegateAccountFlags(paramsAccountDelegate *ParametersAccountsDelegate) (*accountmanager.DelegateAccountParams, error) {
	if paramsAccountDelegate == nil {
		return nil, ierrors.New("paramsAccountDelegate missing for delegate account")
	}

	return &accountmanager.DelegateAccountParams{
		FromAlias: paramsAccountDelegate.FromAlias,
		ToAddress: paramsAccountDelegate.ToAddress,
		Amount:    iotago.BaseToken(paramsAccountDelegate.Amount),
		CheckPool: paramsAccountDelegate.CheckPool,
	}, nil
}

func parseUpdateAccountFlags(paramsAccountUpdate *ParametersAccountsUpdate) (*accountmanager.UpdateAccountParams, error) {
	if paramsAccountUpdate == nil {
		return nil, ierrors.New("paramsAccountUpdate missing for update account")
	}

	return &accountmanager.UpdateAccountParams{
		Alias:          paramsAccountUpdate.Alias,
		BlockIssuerKey: paramsAccountUpdate.BlockIssuerKey,
		Amount:         uint64(paramsAccountUpdate.Amount),
		Mana:           uint64(paramsAccountUpdate.Mana),
		ExpirySlot:     uint64(paramsAccountUpdate.ExpirySlot),
	}, nil
}
