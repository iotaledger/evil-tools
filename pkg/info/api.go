package info

import (
	"context"

	"github.com/iotaledger/evil-tools/pkg/utils"
)

func (m *Manager) RequestCommittee(ctx context.Context) error {
	resp, err := m.accWallet.Client.GetCommittee(ctx)
	if err != nil {
		return err
	}

	m.logger.LogInfo(utils.SprintCommittee(resp))

	return nil
}
