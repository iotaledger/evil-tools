package info

import (
	"context"
	"fmt"

	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
)

func (m *Manager) CommitteeInfo(ctx context.Context) error {
	resp, err := m.accWallets.Client.GetCommittee(ctx)
	if err != nil {
		return err
	}

	m.LogInfo(fmt.Sprintf("### \n%s", utils.SprintCommittee(resp)))

	return nil
}

func (m *Manager) ValidatorsInfo(ctx context.Context) error {
	resp, err := m.accWallets.Client.GetValidators(ctx)
	if err != nil {
		return err
	}

	m.LogInfo(fmt.Sprintf("### \n%s", utils.SprintValidators(resp)))

	return nil
}

func (m *Manager) AccountsInfo() error {
	return m.accWallets.ListAccount()
}

func (m *Manager) DelegatorsInfo() error {
	delegationOutToAliasMap := m.accWallets.Delegators()
	t := "\n### Delegators: \n"
	for _, alias := range delegationOutToAliasMap {
		t += fmt.Sprintf("Alias: %-12s\n", alias)
		delegations, err := m.accWallets.GetDelegations(alias)
		if err != nil {
			m.LogInfof("Could not get delegations for alias %s: %v", alias, err.Error())
		}
		for _, del := range delegations {
			t += fmt.Sprintf("OutputID: %s, Amount: %d BechAddr: %s\n", del.OutputID.ToHex(), del.Amount, del.DelegatedToBechAddress)
		}
	}

	m.LogInfo(t)

	return nil
}

func (m *Manager) RewardsInfo(ctx context.Context, params *ParametersInfo) error {
	aliases := []string{params.Alias}
	if params.Alias == "" {
		aliases = m.accWallets.Delegators()
	}

	out := ""
	for _, alias := range aliases {
		out += "----------\n"
		out += fmt.Sprintf("%-10s %-33s\n", "Alias", alias)
		// first, get the account output if this alias has one and check if it has validator rewards
		accData, err := m.accWallets.GetAccount(params.Alias)
		if err != nil {
			m.LogInfof("Could not get account %s: %v", params.Alias, err)
		} else {
			validatorReward, err := m.accWallets.Client.GetRewards(ctx, accData.OutputID)
			if err != nil {
				m.LogInfof("Could not get rewards for account %s: %v", params.Alias, err)
			} else {
				out += fmt.Sprintf("Staking reward: %d, startEpoch: %d, endEpoch: %d", validatorReward.Rewards, validatorReward.StartEpoch, validatorReward.EndEpoch)
			}
		}

		// next get the rewards for any delegation under this alias
		delegations, err := m.accWallets.GetDelegations(params.Alias)
		if err != nil {
			return ierrors.Wrap(err, "failed to get delegations")
		}
		out += "Delegations:\n"
		for _, delegation := range delegations {
			delegationReward, err := m.accWallets.Client.GetRewards(ctx, delegation.OutputID)
			if err != nil {
				return ierrors.Wrapf(err, "failed to get rewards for output with outputID %s", delegation.OutputID)
			}
			out += fmt.Sprintf("OutputID: %-12s, Reward: %-33d startEpoch: %d, endEpoch: %d", delegation.OutputID.ToHex(), delegationReward.Rewards, delegationReward.StartEpoch, delegationReward.EndEpoch)
		}
	}

	m.LogInfo(out)

	return nil
}
