package info

import (
	"context"

	"github.com/iotaledger/evil-tools/pkg/utils"
)

func (m *Manager) CommitteeInfo(ctx context.Context) error {
	resp, err := m.accWallet.Client.GetCommittee(ctx)
	if err != nil {
		return err
	}

	m.logger.LogInfo(utils.SprintCommittee(resp))

	return nil
}

func (m *Manager) ValidatorsInfo(ctx context.Context) error {
	resp, err := m.accWallet.Client.GetValidators(ctx)
	if err != nil {
		return err
	}

	m.logger.LogInfo(utils.SprintValidators(resp))

	return nil
}

func (m *Manager) AccountsInfo() error {
	return m.accWallet.ListAccount()
}
