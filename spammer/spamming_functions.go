package spammer

import (
	"math/rand"
	"sync"
	"time"

	"github.com/iotaledger/evil-tools/models"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
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

func BlowballSpammingFunction(s *Spammer) {
	clt := s.Clients.GetClient()
	// sleep randomly to avoid issuing blocks in different goroutines at once
	//nolint:gosec
	time.Sleep(time.Duration(rand.Float64()*20) * time.Millisecond)

	centerID, err := createBlowBallCenter(s)
	if err != nil {
		s.log.Errorf("failed to performe blowball attack", err)
		return
	}
	s.log.Infof("blowball center ID: %s", centerID.ToHex())

	// wait for the center block to be an old confirmed block
	s.log.Infof("wait blowball center to get old...")
	time.Sleep(30 * time.Second)

	blowballs := createBlowBall(centerID, s)

	wg := sync.WaitGroup{}
	for _, blk := range blowballs {
		// send transactions in parallel
		wg.Add(1)
		go func(clt models.Client, blk *iotago.ProtocolBlock) {
			defer wg.Done()

			// sleep randomly to avoid issuing blocks in different goroutines at once
			//nolint:gosec
			time.Sleep(time.Duration(rand.Float64()*100) * time.Millisecond)

			id, err := clt.PostBlock(blk)
			if err != nil {
				s.log.Error("ereror to send blowball blocks")
				return
			}
			s.log.Infof("blowball sent, ID: %s", id.ToHex())
		}(clt, blk)
	}
	wg.Wait()

	s.State.batchPrepared.Add(1)
	s.CheckIfAllSent()
}

func createBlowBallCenter(s *Spammer) (iotago.BlockID, error) {
	clt := s.Clients.GetClient()

	centerID := s.PrepareAndPostBlock(&models.PayloadIssuanceData{
		Payload: &iotago.TaggedData{
			Tag: []byte("DS"),
		},
	}, s.IssuerAlias, clt)

	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			state := clt.GetBlockConfirmationState(centerID)
			if state == "confirmed" {
				return centerID, nil
			}
		case <-timer.C:
			return iotago.EmptyBlockID, ierrors.Errorf("failed to confirm center block")
		}
	}
}

func createBlowBall(center iotago.BlockID, s *Spammer) []*iotago.ProtocolBlock {
	blowBallBlocks := make([]*iotago.ProtocolBlock, 0)
	// default to 30, if blowball size is not set
	size := lo.Max(s.SpamDetails.BlowballSize, 30)

	for i := 0; i < size; i++ {
		blk := createSideBlock(center, s)
		blowBallBlocks = append(blowBallBlocks, blk)
	}

	return blowBallBlocks
}

func createSideBlock(parent iotago.BlockID, s *Spammer) *iotago.ProtocolBlock {
	// create a new message
	clt := s.Clients.GetClient()

	return s.PrepareBlock(&models.PayloadIssuanceData{
		Payload: &iotago.TaggedData{
			Tag: []byte("DS"),
		},
	}, s.IssuerAlias, clt, parent)
}
