package info

import (
	"context"
	"fmt"

	"github.com/iotaledger/evil-tools/pkg/utils"
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
			out += fmt.Sprintf("No account found for alias %s\n", params.Alias)
		} else {
			validatorReward, err := m.accWallets.Client.GetRewards(ctx, accData.OutputID)
			if err != nil {
				out += fmt.Sprintf("No staking rewards found for alias %s\n", params.Alias)
			} else {
				out += fmt.Sprintf("Staking reward: %d, startEpoch: %d, endEpoch: %d\n", validatorReward.Rewards, validatorReward.StartEpoch, validatorReward.EndEpoch)
			}
		}

		// next get the rewards for any delegation under this alias
		delegations, err := m.accWallets.GetDelegations(alias)
		if err != nil {
			//m.LogErrorf("failed to get delegations for alias %s", params.Alias)

			continue
		}
		out += "\nDelegations:\n"
		for _, delegation := range delegations {
			delegationReward, err := m.accWallets.Client.GetRewards(ctx, delegation.OutputID)
			if err != nil {
				//m.LogErrorf("failed to get rewards for output with outputID %s", delegation.OutputID)

				continue
			}
			out += fmt.Sprintf("OutputID: %-12s, Reward: %-23d startEpoch: %d, endEpoch: %d\n", delegation.OutputID.ToHex(), delegationReward.Rewards, delegationReward.StartEpoch, delegationReward.EndEpoch)
		}
	}

	m.LogInfo(out)

	return nil
}
