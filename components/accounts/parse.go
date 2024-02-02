package accounts

import (
	"fmt"
	"strings"

	"github.com/iotaledger/evil-tools/pkg/walletmanager"
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

		if !walletmanager.AvailableCommands(arg) {
			// skip as it might be a flag parameter
			continue
		}
		commands = append(commands, arg)
	}

	return commands
}

func parseAccountCommands(commands []string, paramsAccounts *ParametersAccounts) []walletmanager.AccountSubcommands {
	parsedCmds := make([]walletmanager.AccountSubcommands, 0)

	for _, cmd := range commands {
		switch cmd {
		case walletmanager.OperationCreateAccount.String():
			createAccountParams, err := parseCreateAccountParams(&paramsAccounts.Create)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, createAccountParams)

		case walletmanager.OperationConvertAccount.String():
			convertAccountParams := parseConvertAccountFlags(&paramsAccounts.Convert)
			parsedCmds = append(parsedCmds, convertAccountParams)

		case walletmanager.OperationDestroyAccount.String():
			destroyAccountParams, err := parseDestroyAccountFlags(&paramsAccounts.Destroy)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, destroyAccountParams)

		case walletmanager.OperationAllotAccount.String():
			allotAccountParams, err := parseAllotAccountFlags(&paramsAccounts.Allot)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, allotAccountParams)

		case walletmanager.OperationDelegate.String():
			delegatingAccountParams, err := parseDelegateAccountFlags(&paramsAccounts.Delegate)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, delegatingAccountParams)

		case walletmanager.OperationStakeAccount.String():
			stakingAccountParams, err := parseStakeAccountFlags(&paramsAccounts.Stake)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, stakingAccountParams)

		case walletmanager.OperationClaim.String():
			rewardsParams, err := parseRewardsFlags(&paramsAccounts.Claim)
			if err != nil {
				continue
			}
			parsedCmds = append(parsedCmds, rewardsParams)

		case walletmanager.OperationUpdateAccount.String():
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
	fmt.Printf("COMMAND: %s\n", walletmanager.OperationCreateAccount)
	_, _ = parseCreateAccountParams(nil)

	fmt.Printf("COMMAND: %s\n", walletmanager.OperationConvertAccount)
	_ = parseConvertAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", walletmanager.OperationDestroyAccount)
	_, _ = parseDestroyAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", walletmanager.OperationAllotAccount)
	_, _ = parseAllotAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", walletmanager.OperationDelegate)
	_, _ = parseDelegateAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", walletmanager.OperationStakeAccount)
	_, _ = parseStakeAccountFlags(nil)

	fmt.Printf("COMMAND: %s\n", walletmanager.OperationClaim)
	_, _ = parseRewardsFlags(nil)
}

func parseCreateAccountParams(paramsAccountCreate *ParametersAccountsCreate) (*walletmanager.CreateAccountParams, error) {
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

	return &walletmanager.CreateAccountParams{
		Alias:      paramsAccountCreate.Alias,
		NoBIF:      paramsAccountCreate.NoBlockIssuerFeature,
		Implicit:   paramsAccountCreate.Implicit,
		Transition: paramsAccountCreate.Transition,
	}, nil
}

func parseConvertAccountFlags(paramsAccountConvert *ParametersAccountsConvert) *walletmanager.ConvertAccountParams {
	return &walletmanager.ConvertAccountParams{
		AccountAlias: paramsAccountConvert.Alias,
	}
}

func parseDestroyAccountFlags(paramsAccountDestroy *ParametersAccountsDestroy) (*walletmanager.DestroyAccountParams, error) {
	if paramsAccountDestroy == nil {
		return nil, ierrors.New("paramsAccountDestroy missing for destroy account")
	}

	return &walletmanager.DestroyAccountParams{
		AccountAlias: paramsAccountDestroy.Alias,
		ExpirySlot:   uint64(paramsAccountDestroy.ExpirySlot),
	}, nil
}

func parseAllotAccountFlags(paramsAccountAllot *ParametersAccountsAllot) (*walletmanager.AllotAccountParams, error) {
	if paramsAccountAllot == nil {
		return nil, ierrors.New("paramsAccountAllot missing for allot account")
	}

	return &walletmanager.AllotAccountParams{
		Alias:  paramsAccountAllot.Alias,
		Amount: iotago.Mana(paramsAccountAllot.Amount),
	}, nil
}

func parseStakeAccountFlags(paramsAccountStake *ParametersAccountsStake) (*walletmanager.StakeAccountParams, error) {
	if paramsAccountStake == nil {
		return nil, ierrors.New("paramsAccountStake missing for stake account")
	}

	return &walletmanager.StakeAccountParams{
		Alias:      paramsAccountStake.Alias,
		Amount:     uint64(paramsAccountStake.Amount),
		FixedCost:  uint64(paramsAccountStake.FixedCost),
		StartEpoch: uint64(paramsAccountStake.StartEpoch),
		EndEpoch:   uint64(paramsAccountStake.EndEpoch),
	}, nil
}

func parseRewardsFlags(paramsRewards *ParametersClaim) (*walletmanager.ClaimAccountParams, error) {
	if paramsRewards == nil {
		return nil, ierrors.New("paramsRewards missing for rewards account")
	}

	return &walletmanager.ClaimAccountParams{
		Alias: paramsRewards.Alias,
	}, nil
}

func parseDelegateAccountFlags(paramsAccountDelegate *ParametersAccountsDelegate) (*walletmanager.DelegateAccountParams, error) {
	if paramsAccountDelegate == nil {
		return nil, ierrors.New("paramsAccountDelegate missing for delegate account")
	}

	return &walletmanager.DelegateAccountParams{
		FromAlias: paramsAccountDelegate.FromAlias,
		ToAddress: paramsAccountDelegate.ToAddress,
		Amount:    iotago.BaseToken(paramsAccountDelegate.Amount),
		CheckPool: paramsAccountDelegate.CheckPool,
	}, nil
}

func parseUpdateAccountFlags(paramsAccountUpdate *ParametersAccountsUpdate) (*walletmanager.UpdateAccountParams, error) {
	if paramsAccountUpdate == nil {
		return nil, ierrors.New("paramsAccountUpdate missing for update account")
	}

	return &walletmanager.UpdateAccountParams{
		Alias:          paramsAccountUpdate.Alias,
		BlockIssuerKey: paramsAccountUpdate.BlockIssuerKey,
		Amount:         uint64(paramsAccountUpdate.Amount),
		Mana:           uint64(paramsAccountUpdate.Mana),
		ExpirySlot:     uint64(paramsAccountUpdate.ExpirySlot),
	}, nil
}
