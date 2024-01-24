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

	m.logger.LogInfo(fmt.Sprintf("### \n%s", utils.SprintCommittee(resp)))

	return nil
}

func (m *Manager) ValidatorsInfo(ctx context.Context) error {
	resp, err := m.accWallets.Client.GetValidators(ctx)
	if err != nil {
		return err
	}

	m.logger.LogInfo(fmt.Sprintf("### \n%s", utils.SprintValidators(resp)))

	return nil
}

func (m *Manager) AccountsInfo() error {
	return m.accWallets.ListAccount()
}

func (m *Manager) DelegatorsInfo(ctx context.Context) error {
	delegationOutToAliasMap := m.accWallets.Delegators()
	t := "### Delegators: \n"
	for outID, alias := range delegationOutToAliasMap {
		t += fmt.Sprintf("OutputID: %-12s, Alias: %-33s", outID, alias)
	}
	m.logger.LogInfo(t)

	return nil
}
