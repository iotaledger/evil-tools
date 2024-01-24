package info

import (
	"github.com/iotaledger/evil-tools/pkg/accountmanager"
	"github.com/iotaledger/hive.go/log"
)

type Manager struct {
	accWallets *accountmanager.Manager
	logger     log.Logger
}

func NewManager(logger log.Logger, accWallet *accountmanager.Manager) *Manager {
	return &Manager{
		accWallets: accWallet,
		logger:     logger,
	}
}
