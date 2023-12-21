package programs

import (
	"context"

	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/evil-tools/pkg/spammer"
	"github.com/iotaledger/hive.go/log"
)

type Runner struct {
	paramsSpammer *spammer.ParametersSpammer

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

func (d *Dispatcher) RunSpam(ctx context.Context, logger log.Logger, nodeURLs []string, paramsSpammer *spammer.ParametersSpammer) {
	CustomSpam(ctx, logger, nodeURLs, paramsSpammer, d.accWallet)

	d.activeSpammers = append(d.activeSpammers, &Runner{
		finished:      make(chan bool),
		paramsSpammer: paramsSpammer,
	})
}
