package spammer

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

func DataSpammingFunction(ctx context.Context, s *Spammer) error {
	clt := s.Clients.GetClient()
	// sleep randomly to avoid issuing blocks in different goroutines at once
	//nolint:gosec
	time.Sleep(time.Duration(rand.Float64()*20) * time.Millisecond)

	s.PrepareAndPostBlock(ctx, &models.PayloadIssuanceData{
		Payload: &iotago.TaggedData{
			Tag: []byte("SPAM"),
		},
	}, s.IssuerAlias, clt)

	s.State.batchPrepared.Add(1)
	s.CheckIfAllSent()

	return nil
}

func CustomConflictSpammingFunc(ctx context.Context, s *Spammer) error {
	conflictBatch, aliases, err := s.EvilWallet.PrepareCustomConflictsSpam(ctx, s.EvilScenario)
	if err != nil {
		s.log.Debugf(ierrors.Wrap(ErrFailToPrepareBatch, err.Error()).Error())
		s.ErrCounter.CountError(ierrors.Wrap(ErrFailToPrepareBatch, err.Error()))

		return err
	}

	//  TODO do we want to use allotment strategy different than All? Maybe to test blocking account...
	//issuanceAndAllotmentStrategy := &models.IssuancePaymentStrategy{
	//	AllotmentStrategy: models.AllotmentStrategyAll,
	//	IssuerAlias:       s.IssuerAlias,
	//}

	for _, payloadsIssuanceData := range conflictBatch {
		clients := s.Clients.GetClients(len(payloadsIssuanceData))
		if len(payloadsIssuanceData) > len(clients) {
			s.log.Debug(ErrFailToPrepareBatch)
			s.ErrCounter.CountError(ErrInsufficientClients)
		}

		// send transactions in parallel
		wg := sync.WaitGroup{}
		for i, issuanceData := range payloadsIssuanceData {
			wg.Add(1)
			go func(clt models.Client, tx *models.PayloadIssuanceData) {
				defer wg.Done()

				// sleep randomly to avoid issuing blocks in different goroutines at once
				//nolint:gosec
				time.Sleep(time.Duration(rand.Float64()*100) * time.Millisecond)

				s.PrepareAndPostBlock(ctx, tx, s.IssuerAlias, clt)
			}(clients[i], issuanceData)
		}
		wg.Wait()
	}
	s.State.batchPrepared.Add(1)
	s.EvilWallet.ClearAliases(aliases)
	s.CheckIfAllSent()

	return nil
}

func AccountSpammingFunction(ctx context.Context, s *Spammer) error {
	clt := s.Clients.GetClient()
	// update scenario
	issuanceData, aliases, err := s.EvilWallet.PrepareAccountSpam(ctx, s.EvilScenario)
	if err != nil {
		s.log.Debugf(ierrors.Wrap(ErrFailToPrepareBatch, err.Error()).Error())
		s.ErrCounter.CountError(ierrors.Wrap(ErrFailToPrepareBatch, err.Error()))

		return err
	}
	s.PrepareAndPostBlock(ctx, issuanceData, s.IssuerAlias, clt)

	s.State.batchPrepared.Add(1)
	s.EvilWallet.ClearAliases(aliases)
	s.CheckIfAllSent()

	return nil
}

func BlowballSpammingFunction(ctx context.Context, s *Spammer) error {
	clt := s.Clients.GetClient()
	// sleep randomly to avoid issuing blocks in different goroutines at once
	//nolint:gosec
	time.Sleep(time.Duration(rand.Float64()*20) * time.Millisecond)

	centerID, err := createBlowBallCenter(ctx, s)
	if err != nil {
		s.log.Errorf("failed to performe blowball attack", err)
		return err
	}
	s.log.Infof("blowball center ID: %s", centerID.ToHex())

	// wait for the center block to be an old confirmed block
	s.log.Infof("wait blowball center to get old...")
	time.Sleep(30 * time.Second)

	blowballs := createBlowBall(ctx, centerID, s)

	wg := sync.WaitGroup{}
	for _, blk := range blowballs {
		// send transactions in parallel
		wg.Add(1)
		go func(clt models.Client, blk *iotago.Block) {
			defer wg.Done()

			// sleep randomly to avoid issuing blocks in different goroutines at once
			//nolint:gosec
			time.Sleep(time.Duration(rand.Float64()*100) * time.Millisecond)

			id, err := clt.PostBlock(ctx, blk)
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

	return nil
}

func createBlowBallCenter(ctx context.Context, s *Spammer) (iotago.BlockID, error) {
	clt := s.Clients.GetClient()

	centerID := s.PrepareAndPostBlock(ctx, &models.PayloadIssuanceData{
		Payload: &iotago.TaggedData{
			Tag: []byte("DS"),
		},
	}, s.IssuerAlias, clt)

	err := utils.AwaitBlockAndPayloadAcceptance(ctx, clt, centerID)

	return centerID, err
}

func createBlowBall(ctx context.Context, center iotago.BlockID, s *Spammer) []*iotago.Block {
	blowBallBlocks := make([]*iotago.Block, 0)
	// default to 30, if blowball size is not set
	size := lo.Max(s.SpamDetails.BlowballSize, 30)

	for i := 0; i < size; i++ {
		blk := createSideBlock(ctx, center, s)
		blowBallBlocks = append(blowBallBlocks, blk)
	}

	return blowBallBlocks
}

func createSideBlock(ctx context.Context, parent iotago.BlockID, s *Spammer) *iotago.Block {
	// create a new message
	clt := s.Clients.GetClient()

	return s.PrepareBlock(ctx, &models.PayloadIssuanceData{
		Payload: &iotago.TaggedData{
			Tag: []byte("DS"),
		},
	}, s.IssuerAlias, clt, parent)
}
