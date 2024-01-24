package spammer

import (
	"context"
	"time"

	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/pkg/accountmanager"
	"github.com/iotaledger/evil-tools/pkg/evilwallet"
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	TypeBlock    = "blk"
	TypeTx       = "tx"
	TypeDs       = "ds"
	TypeAccounts = "accounts"
	TypeBlowball = "bb"
)

const (
	InfiniteDuration = time.Duration(-1)
)

// region Spammer //////////////////////////////////////////////////////////////////////////////////////////////////////

type SpammingFunc func(context.Context, *Spammer) error

type State struct {
	spamTicker    *time.Ticker
	logTicker     *time.Ticker
	spamStartTime time.Time
	blkSent       *atomic.Int64
	batchPrepared *atomic.Int64

	logTickTime  time.Duration
	spamDuration time.Duration
}

type SpamType int

const (
	SpamEvilWallet SpamType = iota
)

// Spammer is a utility object for new spammer creations, can be modified by passing options.
// Mandatory options: WithClients, WithSpammingFunc
// Not mandatory options, if not provided spammer will use default settings:
// WithSpamDetails, WithEvilWallet, WithErrorCounter, WithLogTickerInterval.
type Spammer struct {
	log.Logger

	State      *State
	Clients    models.Connector
	ErrCounter *ErrorCounter

	// accessed from spamming functions
	done   chan bool
	failed chan bool

	MaxBatchesSent int
	NumberOfSpends int

	// options
	EvilWallet    *evilwallet.EvilWallet
	EvilScenario  *evilwallet.EvilScenario
	spammingFunc  SpammingFunc
	IssuerAlias   string
	UseRateSetter bool
	SpamType      SpamType
	Rate          int
	MaxDuration   time.Duration
	BlowballSize  int
}

// NewSpammer is a constructor of Spammer.
func NewSpammer(logger log.Logger, opts ...options.Option[Spammer]) *Spammer {
	state := &State{
		blkSent:       atomic.NewInt64(0),
		batchPrepared: atomic.NewInt64(0),
		logTickTime:   time.Second * 30,
	}

	spammer := options.Apply(&Spammer{
		Logger:         logger.NewChildLogger("Spammer"),
		spammingFunc:   CustomConflictSpammingFunc,
		State:          state,
		SpamType:       SpamEvilWallet,
		EvilScenario:   evilwallet.NewEvilScenario(),
		UseRateSetter:  true,
		done:           make(chan bool),
		failed:         make(chan bool),
		NumberOfSpends: 2,
	}, opts)

	spammer.setup()

	return spammer
}

func (s *Spammer) BlocksSent() uint64 {
	return uint64(s.State.blkSent.Load())
}

func (s *Spammer) BatchesPrepared() uint64 {
	return uint64(s.State.batchPrepared.Load())
}

func (s *Spammer) setup() {
	switch s.SpamType {
	case SpamEvilWallet:
		if s.EvilWallet == nil {
			s.EvilWallet = evilwallet.NewEvilWallet(s.Logger)
		}
		s.Clients = s.EvilWallet.Connector()
		// case SpamCommitments:
		// 	s.CommitmentManager.Setup(s.log)
	}
	s.setupSpamDetails()

	s.State.spamTicker = s.initSpamTicker()
	s.State.logTicker = s.initLogTicker()

	if s.ErrCounter == nil {
		s.ErrCounter = NewErrorCount()
	}
}

func (s *Spammer) setupSpamDetails() {
	if s.Rate <= 0 {
		s.Rate = 1
	}

	// provided only maxDuration, calculating the default max for maxBlkSent
	if s.MaxDuration > 0 {
		s.MaxBatchesSent = int(s.MaxDuration.Seconds()*float64(s.Rate)) + 1
	}

	if s.IssuerAlias == "" {
		s.IssuerAlias = accountmanager.GenesisAccountAlias
	}
}

func (s *Spammer) initSpamTicker() *time.Ticker {
	tickerTime := float64(time.Second) / float64(s.Rate)
	return time.NewTicker(time.Duration(tickerTime))
}

func (s *Spammer) initLogTicker() *time.Ticker {
	return time.NewTicker(s.State.logTickTime)
}

// Spam runs the spammer. Function will stop after maxDuration time will pass or when maxBlkSent will be exceeded.
func (s *Spammer) Spam(ctx context.Context) {
	s.LogInfof("Start spamming transactions with %d rate", s.Rate)
	defer func() {
		s.LogInfo(s.ErrCounter.GetErrorsSummary())
		s.LogInfof("Finishing spamming, total txns sent: %v, TotalTime: %v, Rate: %f", s.State.blkSent.Load(), s.State.spamDuration.Seconds(), float64(s.State.blkSent.Load())/s.State.spamDuration.Seconds())
	}()

	s.State.spamStartTime = time.Now()
	var newContext context.Context
	var cancel context.CancelFunc

	if s.MaxDuration > 0 {
		newContext, cancel = context.WithDeadline(ctx, s.State.spamStartTime.Add(s.MaxDuration))
	} else {
		newContext, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	go func(newContext context.Context, s *Spammer) {
		goroutineCount := atomic.NewInt32(0)
		for {
			select {
			case <-s.State.logTicker.C:
				s.LogInfof("Blocks issued so far: %d, errors encountered: %d", s.State.blkSent.Load(), s.ErrCounter.GetTotalErrorCount())
			case <-ctx.Done():
				s.LogInfo("Maximum spam duration exceeded, stopping spammer....")
				return
			case <-s.State.spamTicker.C:
				if goroutineCount.Load() > 100 {
					break
				}
				go func(newContext context.Context, s *Spammer) {
					goroutineCount.Inc()
					defer goroutineCount.Dec()

					err := s.spammingFunc(newContext, s)
					// currently we stop the spammer when there's no fresh faucet outputs.
					if ierrors.Is(err, evilwallet.NoFreshOutputsAvailable) {
						s.failed <- true
					}
				}(newContext, s)
			}
		}
	}(newContext, s)

	// await for shutdown signal
	for {
		select {
		case <-s.done:
			s.StopSpamming()
			return
		case <-s.failed:
			s.StopSpamming()
			return
		}
	}
}

func (s *Spammer) logError(err error) {
	if ierrors.Is(err, context.DeadlineExceeded) {
		// ignore deadline exceeded errors as the spammer has sent thew signal to stop
		return
	}

	s.LogDebug(err.Error())
}

func (s *Spammer) CheckIfAllSent() {
	if s.MaxDuration >= 0 && s.State.batchPrepared.Load() >= int64(s.MaxBatchesSent) {
		s.LogInfo("Maximum number of blocks sent, stopping spammer...")
		s.done <- true
	}
}

// StopSpamming finishes tasks before shutting down the spammer.
func (s *Spammer) StopSpamming() {
	s.State.spamDuration = time.Since(s.State.spamStartTime)
	s.State.spamTicker.Stop()
	s.State.logTicker.Stop()
}

func (s *Spammer) PrepareBlock(ctx context.Context, issuanceData *models.PayloadIssuanceData, clt models.Client, strongParents ...iotago.BlockID) *iotago.Block {
	if issuanceData.Payload == nil {
		s.logError(ErrPayloadIsNil)
		s.ErrCounter.CountError(ErrPayloadIsNil)

		return nil
	}
	issuerAccount, err := s.EvilWallet.GetAccount(ctx, s.IssuerAlias)
	if err != nil {
		s.logError(ierrors.Wrap(err, ErrFailGetAccount.Error()))
		s.ErrCounter.CountError(ierrors.Wrap(err, ErrFailGetAccount.Error()))

		return nil
	}
	block, err := s.EvilWallet.CreateBlock(ctx, clt, issuanceData.Payload, issuerAccount, strongParents...)
	if err != nil {
		s.logError(ierrors.Wrap(err, ErrFailPostBlock.Error()))
		s.ErrCounter.CountError(ierrors.Wrap(err, ErrFailPostBlock.Error()))

		return nil
	}

	return block
}

func (s *Spammer) PrepareAndPostBlock(ctx context.Context, issuanceData *models.PayloadIssuanceData, clt models.Client) iotago.BlockID {
	if issuanceData.Payload == nil && issuanceData.TransactionBuilder == nil {
		s.logError(ErrPayloadIsNil)
		s.ErrCounter.CountError(ErrPayloadIsNil)

		return iotago.EmptyBlockID
	}
	issuerAccount, err := s.EvilWallet.GetAccount(ctx, s.IssuerAlias)
	if err != nil {
		s.logError(ierrors.Wrap(err, ErrFailGetAccount.Error()))
		s.ErrCounter.CountError(ierrors.Wrap(err, ErrFailGetAccount.Error()))

		return iotago.EmptyBlockID
	}

	var blockID iotago.BlockID
	var tx *iotago.Transaction
	// built, allot and sign transaction or issue a ready payload
	switch issuanceData.Type {
	case iotago.PayloadTaggedData:
		blockID, err = s.EvilWallet.PrepareAndPostBlockWithPayload(ctx, clt, issuanceData.Payload, issuerAccount)
	case iotago.PayloadSignedTransaction:
		blockID, tx, err = s.EvilWallet.PrepareAndPostBlockWithTxBuildData(ctx, clt, issuanceData.TransactionBuilder, issuanceData.TxSigningKeys, issuerAccount)
	default:
		// unknown payload type
		s.logError(ErrUnknownPayloadType)
		s.ErrCounter.CountError(ErrUnknownPayloadType)

		return iotago.EmptyBlockID
	}

	if err != nil {
		s.logError(ierrors.Wrap(err, ErrFailPostBlock.Error()))
		s.ErrCounter.CountError(ierrors.Wrap(err, ErrFailPostBlock.Error()))

		return iotago.EmptyBlockID
	}
	s.LogDebugf("Issued block, blockID %s, issuer %s", blockID.ToHex(), issuerAccount.ID().ToHex())

	if issuanceData.Type == iotago.PayloadSignedTransaction {
		// reuse outputs
		if s.EvilScenario.OutputWallet.Type() == evilwallet.Reuse {
			txID, err := tx.ID()
			if err != nil {
				s.logError(ierrors.Wrap(err, ErrFailPostBlock.Error()))
				s.ErrCounter.CountError(ierrors.Wrap(err, ErrFailPostBlock.Error()))

				return iotago.EmptyBlockID
			}

			var outputIDs iotago.OutputIDs
			for index := range tx.Outputs {
				outputIDs = append(outputIDs, iotago.OutputIDFromTransactionIDAndIndex(txID, uint16(index)))
			}

			s.EvilWallet.SetTxOutputsSolid(outputIDs, clt.URL())
		}
	}

	if issuanceData.Type != iotago.PayloadSignedTransaction {
		count := s.State.blkSent.Add(1)
		if count%200 == 0 {
			s.LogInfof("Blocks issued so far: %d, errors encountered: %d", count, s.ErrCounter.GetTotalErrorCount())
		}

		return blockID
	}

	count := s.State.blkSent.Add(1)
	//s.log.Debugf("Last block sent, ID: %s, txCount: %d", blockID.ToHex(), count)
	if count%200 == 0 {
		s.LogInfof("Blocks issued so far: %d, errors encountered: %d", count, s.ErrCounter.GetTotalErrorCount())
	}

	return blockID
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
