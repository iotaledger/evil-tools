package accountwallet

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
)

func (a *AccountWallets) rewards(ctx context.Context, params *RewardsAccountParams) error {
	// first, get the account output if this alias has one and check if it has validator rewards
	accData, err := a.GetAccount(params.Alias)
	if err != nil {
		a.LogInfof("Could not get account %s: %v", params.Alias, err)
	} else {
		validatorReward, err := a.client.GetRewards(ctx, accData.OutputID)
		if err != nil {
			a.LogInfof("Could not get rewards for account %s: %v", params.Alias, err)
		} else {
			a.LogInfof("Account %s has %d Mana rewards ready to be claimed", params.Alias, validatorReward.Rewards)
		}
	}
	// next get the rewards for any delegation under this alias
	delegations, err := a.GetDelegations(params.Alias)
	if err != nil {
		return ierrors.Wrap(err, "failed to get delegations")
	}
	for _, delegationOutput := range delegations {
		delegationReward, err := a.client.GetRewards(ctx, delegationOutput.OutputID)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get rewards for output with outputID %s", delegationOutput.OutputID)
		}
		a.LogInfof("Delegation with outputID %s has %d Mana rewards ready to be claimed", delegationOutput.OutputID, delegationReward.Rewards)
	}

	return nil
}
