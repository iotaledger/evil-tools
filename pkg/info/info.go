package info

import (
	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/hive.go/log"
)

type Manager struct {
	accWallet *accountwallet.AccountWallet
	logger    log.Logger
}

func NewManager(logger log.Logger, accWallet *accountwallet.AccountWallet) *Manager {
	return &Manager{
		accWallet: accWallet,
		logger:    logger,
	}
}
