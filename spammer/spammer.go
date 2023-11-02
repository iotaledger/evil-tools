package spammer

import (
	"time"

	"go.uber.org/atomic"

	"github.com/iotaledger/evil-tools/evilwallet"
	"github.com/iotaledger/evil-tools/models"
	"github.com/iotaledger/hive.go/app/configuration"
	appLogger "github.com/iotaledger/hive.go/app/logger"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/tools/genesis-snapshot/presets"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	TypeBlock    = "blk"
	TypeTx       = "tx"
	TypeDs       = "ds"
	TypeCustom   = "custom"
	TypeAccounts = "accounts"
	TypeBlowball = "bb"
)

const (
	InfiniteDuration = time.Duration(-1)
)

// region Spammer //////////////////////////////////////////////////////////////////////////////////////////////////////

type SpammingFunc func(*Spammer)

type State struct {
	spamTicker    *time.Ticker
	logTicker     *time.Ticker
	spamStartTime time.Time
	txSent        *atomic.Int64
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
	SpamDetails   *SpamDetails
	State         *State
	UseRateSetter bool
	SpamType      SpamType
	Clients       models.Connector
	EvilWallet    *evilwallet.EvilWallet
	EvilScenario  *evilwallet.EvilScenario
	// CommitmentManager *CommitmentManager
	ErrCounter  *ErrorCounter
	IssuerAlias string

	log Logger
	api iotago.API

	// accessed from spamming functions
	done         chan bool
	shutdown     chan types.Empty
	spammingFunc SpammingFunc

	TimeDelayBetweenConflicts time.Duration
	NumberOfSpends            int
}

// NewSpammer is a constructor of Spammer.
func NewSpammer(options ...Options) *Spammer {
	protocolParams := snapshotcreator.NewOptions(presets.Docker...).ProtocolParameters
	api := iotago.V3API(protocolParams)

	state := &State{
		txSent:        atomic.NewInt64(0),
		batchPrepared: atomic.NewInt64(0),
		logTickTime:   time.Second * 30,
	}
	s := &Spammer{
		SpamDetails:  &SpamDetails{},
		spammingFunc: CustomConflictSpammingFunc,
		State:        state,
		SpamType:     SpamEvilWallet,
		EvilScenario: evilwallet.NewEvilScenario(),
		// CommitmentManager: NewCommitmentManager(),
		UseRateSetter:  true,
		done:           make(chan bool),
		shutdown:       make(chan types.Empty),
		NumberOfSpends: 2,
		api:            api,
	}

	for _, opt := range options {
		opt(s)
	}

	s.setup()

	return s
}

func (s *Spammer) BlocksSent() uint64 {
	return uint64(s.State.txSent.Load())
}

func (s *Spammer) BatchesPrepared() uint64 {
	return uint64(s.State.batchPrepared.Load())
}

func (s *Spammer) setup() {
	if s.log == nil {
		s.initLogger()
	}

	switch s.SpamType {
	case SpamEvilWallet:
		if s.EvilWallet == nil {
			s.EvilWallet = evilwallet.NewEvilWallet()
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
	if s.SpamDetails.Rate <= 0 {
		s.SpamDetails.Rate = 1
	}
	if s.SpamDetails.TimeUnit == 0 {
		s.SpamDetails.TimeUnit = time.Second
	}
	// provided only maxDuration, calculating the default max for maxBlkSent
	if s.SpamDetails.MaxDuration > 0 {
		s.SpamDetails.MaxBatchesSent = int(s.SpamDetails.MaxDuration.Seconds()/s.SpamDetails.TimeUnit.Seconds()*float64(s.SpamDetails.Rate)) + 1
	}
}

func (s *Spammer) initLogger() {
	config := configuration.New()
	_ = appLogger.InitGlobalLogger(config)
	logger.SetLevel(logger.LevelDebug)
	s.log = logger.NewLogger("Spammer")
}

func (s *Spammer) initSpamTicker() *time.Ticker {
	tickerTime := float64(s.SpamDetails.TimeUnit) / float64(s.SpamDetails.Rate)
	return time.NewTicker(time.Duration(tickerTime))
}

func (s *Spammer) initLogTicker() *time.Ticker {
	return time.NewTicker(s.State.logTickTime)
}

// Spam runs the spammer. Function will stop after maxDuration time will pass or when maxBlkSent will be exceeded.
func (s *Spammer) Spam() {
	s.log.Infof("Start spamming transactions with %d rate", s.SpamDetails.Rate)

	s.State.spamStartTime = time.Now()

	var timeExceeded <-chan time.Time
	// if duration less than zero then spam infinitely
	if s.SpamDetails.MaxDuration >= 0 {
		timeExceeded = time.After(s.SpamDetails.MaxDuration)
	}

	go func() {
		goroutineCount := atomic.NewInt32(0)
		for {
			select {
			case <-s.State.logTicker.C:
				s.log.Infof("Blocks issued so far: %d, errors encountered: %d", s.State.txSent.Load(), s.ErrCounter.GetTotalErrorCount())
			case <-timeExceeded:
				s.log.Infof("Maximum spam duration exceeded, stopping spammer....")
				s.StopSpamming()

				return
			case <-s.done:
				s.StopSpamming()
				return
			case <-s.State.spamTicker.C:
				if goroutineCount.Load() > 100 {
					break
				}
				go func() {
					goroutineCount.Inc()
					defer goroutineCount.Dec()
					s.spammingFunc(s)
				}()
			}
		}
	}()
	<-s.shutdown
	s.log.Info(s.ErrCounter.GetErrorsSummary())
	s.log.Infof("Finishing spamming, total txns sent: %v, TotalTime: %v, Rate: %f", s.State.txSent.Load(), s.State.spamDuration.Seconds(), float64(s.State.txSent.Load())/s.State.spamDuration.Seconds())
}

func (s *Spammer) CheckIfAllSent() {
	if s.SpamDetails.MaxDuration >= 0 && s.State.batchPrepared.Load() >= int64(s.SpamDetails.MaxBatchesSent) {
		s.log.Infof("Maximum number of blocks sent, stopping spammer...")
		s.done <- true
	}
}

// StopSpamming finishes tasks before shutting down the spammer.
func (s *Spammer) StopSpamming() {
	s.State.spamDuration = time.Since(s.State.spamStartTime)
	s.State.spamTicker.Stop()
	s.State.logTicker.Stop()
	// s.CommitmentManager.Shutdown()
	s.shutdown <- types.Void
}

func (s *Spammer) PrepareBlock(txData *models.PayloadIssuanceData, issuerAlias string, clt models.Client, strongParents ...iotago.BlockID) *iotago.ProtocolBlock {
	if txData.Payload == nil {
		s.log.Debug(ErrPayloadIsNil)
		s.ErrCounter.CountError(ErrPayloadIsNil)

		return nil
	}
	issuerAccount, err := s.EvilWallet.GetAccount(issuerAlias)
	if err != nil {
		s.log.Debug(ierrors.Wrapf(ErrFailGetAccount, err.Error()))
		s.ErrCounter.CountError(ierrors.Wrapf(ErrFailGetAccount, err.Error()))

		return nil
	}
	block, err := s.EvilWallet.CreateBlock(clt, txData.Payload, txData.CongestionResponse, issuerAccount, strongParents...)
	if err != nil {
		s.log.Debug(ierrors.Wrapf(ErrFailPostBlock, err.Error()))
		s.ErrCounter.CountError(ierrors.Wrapf(ErrFailPostBlock, err.Error()))

		return nil
	}

	return block
}

func (s *Spammer) PrepareAndPostBlock(txData *models.PayloadIssuanceData, issuerAlias string, clt models.Client) iotago.BlockID {
	if txData.Payload == nil {
		s.log.Debug(ErrPayloadIsNil)
		s.ErrCounter.CountError(ErrPayloadIsNil)

		return iotago.EmptyBlockID
	}
	issuerAccount, err := s.EvilWallet.GetAccount(issuerAlias)
	if err != nil {
		s.log.Debug(ierrors.Wrapf(ErrFailGetAccount, err.Error()))
		s.ErrCounter.CountError(ierrors.Wrapf(ErrFailGetAccount, err.Error()))

		return iotago.EmptyBlockID
	}
	blockID, err := s.EvilWallet.PrepareAndPostBlock(clt, txData.Payload, txData.CongestionResponse, issuerAccount)
	if err != nil {
		s.log.Debug(ierrors.Wrapf(ErrFailPostBlock, err.Error()))
		s.ErrCounter.CountError(ierrors.Wrapf(ErrFailPostBlock, err.Error()))

		return iotago.EmptyBlockID
	}

	if txData.Payload.PayloadType() != iotago.PayloadSignedTransaction {
		return blockID
	}

	//nolint:all,forcetypassert
	signedTx := txData.Payload.(*iotago.SignedTransaction)

	txID, err := signedTx.Transaction.ID()
	if err != nil {
		s.log.Debug(ierrors.Wrapf(ErrTransactionInvalid, err.Error()))
		s.ErrCounter.CountError(ierrors.Wrapf(ErrTransactionInvalid, err.Error()))

		return blockID
	}

	// reuse outputs
	if txData.Payload.PayloadType() == iotago.PayloadSignedTransaction {
		if s.EvilScenario.OutputWallet.Type() == evilwallet.Reuse {
			var outputIDs iotago.OutputIDs
			for index := range signedTx.Transaction.Outputs {
				outputIDs = append(outputIDs, iotago.OutputIDFromTransactionIDAndIndex(txID, uint16(index)))
			}
			s.EvilWallet.SetTxOutputsSolid(outputIDs, clt.URL())
		}
	}
	count := s.State.txSent.Add(1)
	//s.log.Debugf("Last block sent, ID: %s, txCount: %d", blockID.ToHex(), count)
	if count%200 == 0 {
		s.log.Infof("Blocks issued so far: %d, errors encountered: %d", count, s.ErrCounter.GetTotalErrorCount())
	}

	return blockID
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

type Logger interface {
	Infof(template string, args ...interface{})
	Info(args ...interface{})
	Debugf(template string, args ...interface{})
	Debug(args ...interface{})
	Warn(args ...interface{})
	Warnf(template string, args ...interface{})
	Error(args ...interface{})
	Errorf(template string, args ...interface{})
}
