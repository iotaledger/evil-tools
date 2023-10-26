package spammer

import (
	"math/rand"
	"sync"
	"time"

	"github.com/iotaledger/evil-tools/models"
	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

func DataSpammingFunction(s *Spammer) {
	clt := s.Clients.GetClient()
	// sleep randomly to avoid issuing blocks in different goroutines at once
	//nolint:gosec
	time.Sleep(time.Duration(rand.Float64()*20) * time.Millisecond)

	s.PrepareAndPostBlock(&models.PayloadIssuanceData{
		Payload: &iotago.TaggedData{
			Tag: []byte("SPAM"),
		},
	}, s.IssuerAlias, clt)

	s.State.batchPrepared.Add(1)
	s.CheckIfAllSent()
}

func CustomConflictSpammingFunc(s *Spammer) {
	conflictBatch, aliases, err := s.EvilWallet.PrepareCustomConflictsSpam(s.EvilScenario, &models.IssuancePaymentStrategy{
		AllotmentStrategy: models.AllotmentStrategyAll,
		IssuerAlias:       s.IssuerAlias,
	})

	if err != nil {
		s.log.Debugf(ierrors.Wrap(ErrFailToPrepareBatch, err.Error()).Error())
		s.ErrCounter.CountError(ierrors.Wrap(ErrFailToPrepareBatch, err.Error()))
	}

	for _, txsData := range conflictBatch {
		clients := s.Clients.GetClients(len(txsData))
		if len(txsData) > len(clients) {
			s.log.Debug(ErrFailToPrepareBatch)
			s.ErrCounter.CountError(ErrInsufficientClients)
		}

		// send transactions in parallel
		wg := sync.WaitGroup{}
		for i, txData := range txsData {
			wg.Add(1)
			go func(clt models.Client, tx *models.PayloadIssuanceData) {
				defer wg.Done()

				// sleep randomly to avoid issuing blocks in different goroutines at once
				//nolint:gosec
				time.Sleep(time.Duration(rand.Float64()*100) * time.Millisecond)

				s.PrepareAndPostBlock(tx, s.IssuerAlias, clt)
			}(clients[i], txData)
		}
		wg.Wait()
	}
	s.State.batchPrepared.Add(1)
	s.EvilWallet.ClearAliases(aliases)
	s.CheckIfAllSent()
}

func AccountSpammingFunction(s *Spammer) {
	clt := s.Clients.GetClient()
	// update scenario
	txData, aliases, err := s.EvilWallet.PrepareAccountSpam(s.EvilScenario, &models.IssuancePaymentStrategy{
		AllotmentStrategy: models.AllotmentStrategyAll,
		IssuerAlias:       s.IssuerAlias,
	})
	if err != nil {
		s.log.Debugf(ierrors.Wrap(ErrFailToPrepareBatch, err.Error()).Error())
		s.ErrCounter.CountError(ierrors.Wrap(ErrFailToPrepareBatch, err.Error()))
	}
	s.PrepareAndPostBlock(txData, s.IssuerAlias, clt)

	s.State.batchPrepared.Add(1)
	s.EvilWallet.ClearAliases(aliases)
	s.CheckIfAllSent()
}
