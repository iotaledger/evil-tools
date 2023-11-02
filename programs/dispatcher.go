package programs

import (
	"github.com/iotaledger/evil-tools/accountwallet"
	"github.com/iotaledger/evil-tools/models"
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

func (d *Dispatcher) RunSpam(params *CustomSpamParams) {
	// todo custom spam should return a spammer instance, and the process should run in the background
	// or we could inject channel to be able to stop the spammer
	CustomSpam(params, d.accWallet)

	d.activeSpammers = append(d.activeSpammers, &Runner{
		finished:    make(chan bool),
		spamDetails: ConfigFromCustomSpamParams(params),
	})
}
