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

// func CommitmentsSpammingFunction(s *Spammer) {
// 	clt := s.Clients.GetClient()
// 	p := payload.NewGenericDataPayload([]byte("SPAM"))
// 	payloadBytes, err := p.Bytes()
// 	if err != nil {
// 		s.ErrCounter.CountError(ErrFailToPrepareBatch)
// 	}
// 	parents, err := clt.GetReferences(payloadBytes, s.CommitmentManager.Params.ParentRefsCount)
// 	if err != nil {
// 		s.ErrCounter.CountError(ErrFailGetReferences)
// 	}
// 	localID := s.IdentityManager.GetIdentity()
// 	commitment, latestConfIndex, err := s.CommitmentManager.GenerateCommitment(clt)
// 	if err != nil {
// 		s.log.Debugf(errors.WithMessage(ErrFailToPrepareBatch, err.Error()).Error())
// 		s.ErrCounter.CountError(errors.WithMessage(ErrFailToPrepareBatch, err.Error()))
// 	}
// 	block := models.NewBlock(
// 		models.WithParents(parents),
// 		models.WithIssuer(localID.PublicKey()),
// 		models.WithIssuingTime(time.Now()),
// 		models.WithPayload(p),
// 		models.WithLatestConfirmedSlot(latestConfIndex),
// 		models.WithCommitment(commitment),
// 		models.WithSignature(ed25519.EmptySignature),
// 	)
// 	signature, err := wallet.SignBlock(block, localID)
// 	if err != nil {
// 		return
// 	}
// 	block.SetSignature(signature)
// 	timeProvider := slot.NewTimeProvider(s.CommitmentManager.Params.GenesisTime.Unix(), int64(s.CommitmentManager.Params.SlotDuration.Seconds()))
// 	if err = block.DetermineID(timeProvider); err != nil {
// 		s.ErrCounter.CountError(ErrFailPrepareBlock)
// 	}
// 	blockBytes, err := block.Bytes()
// 	if err != nil {
// 		s.ErrCounter.CountError(ErrFailPrepareBlock)
// 	}

// 	blkID, err := clt.PostBlock(blockBytes)
// 	if err != nil {
// 		fmt.Println(err)
// 		s.ErrCounter.CountError(ErrFailSendDataBlock)
// 	}

// 	count := s.State.txSent.Add(1)
// 	if count%int64(s.SpamDetails.Rate*4) == 0 {
// 		s.log.Debugf("Last sent block, ID: %s; %s blkCount: %d", blkID, commitment.ID().String(), count)
// 	}
// 	s.State.batchPrepared.Add(1)
// 	s.CheckIfAllSent()
// }
