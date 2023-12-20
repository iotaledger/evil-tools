package programs

import (
	"context"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/log"
)

type Runner struct {
	spamDetails *models.Config

	finished chan bool
}

type Dispatcher struct {
	activeSpammers []*Runner
	accWallet      *accountwallet.AccountWallet
}

func NewDispatcher(accWallet *accountwallet.AccountWallet) *Dispatcher {
	return &Dispatcher{
		accWallet: accWallet,
	}
}

func (d *Dispatcher) RunSpam(ctx context.Context, logger log.Logger, params *CustomSpamParams) {
	CustomSpam(ctx, logger, params, d.accWallet)

	d.activeSpammers = append(d.activeSpammers, &Runner{
		finished:    make(chan bool),
		spamDetails: ConfigFromCustomSpamParams(params),
	})
}
