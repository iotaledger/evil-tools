package accountmanager

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
)

func (m *Manager) rewards(ctx context.Context, params *RewardsAccountParams) error {
	// first, get the account output if this alias has one and check if it has validator rewards
	accData, err := m.GetAccount(params.Alias)
	if err != nil {
		m.LogInfof("Could not get account %s: %v", params.Alias, err)
	} else {
		validatorReward, err := m.Client.GetRewards(ctx, accData.OutputID)
		if err != nil {
			m.LogInfof("Could not get rewards for account %s: %v", params.Alias, err)
		} else {
			m.LogInfof("Account %s has %d Mana rewards ready to be claimed", params.Alias, validatorReward.Rewards)
		}
	}
	// next get the rewards for any delegation under this alias
	delegations, err := m.GetDelegations(params.Alias)
	if err != nil {
		return ierrors.Wrap(err, "failed to get delegations")
	}
	for _, delegationOutput := range delegations {
		delegationReward, err := m.Client.GetRewards(ctx, delegationOutput.OutputID)
		if err != nil {
			return ierrors.Wrapf(err, "failed to get rewards for output with outputID %s", delegationOutput.OutputID)
		}
		m.LogInfof("Delegation with outputID %s has %d Mana rewards ready to be claimed", delegationOutput.OutputID, delegationReward.Rewards)
	}

	return nil
}
