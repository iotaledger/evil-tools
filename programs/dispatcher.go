package programs

import (
	"context"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/evil-tools/pkg/models"
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

func (d *Dispatcher) RunSpam(ctx context.Context, params *CustomSpamParams) {
	CustomSpam(ctx, params, d.accWallet)

	d.activeSpammers = append(d.activeSpammers, &Runner{
		finished:    make(chan bool),
		spamDetails: ConfigFromCustomSpamParams(params),
	})
}
