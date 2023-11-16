package spammer

import (
	"time"

	"github.com/iotaledger/evil-tools/pkg/evilwallet"
)

type Options func(*Spammer)

// region Spammer general options ///////////////////////////////////////////////////////////////////////////////////////////////////

// WithSpamRate provides spammer with options regarding rate, time unit, and finishing spam criteria. Provide 0 to one of max parameters to skip it.
func WithSpamRate(rate int, timeUnit time.Duration) Options {
	return func(s *Spammer) {
		if s.SpamDetails == nil {
			s.SpamDetails = &SpamDetails{
				Rate:     rate,
				TimeUnit: timeUnit,
			}
		} else {
			s.SpamDetails.Rate = rate
			s.SpamDetails.TimeUnit = timeUnit
		}
	}
}

// WithSpamDuration provides spammer with options regarding rate, time unit, and finishing spam criteria. Provide 0 to one of max parameters to skip it.
func WithSpamDuration(maxDuration time.Duration) Options {
	return func(s *Spammer) {
		if s.SpamDetails == nil {
			s.SpamDetails = &SpamDetails{
				MaxDuration: maxDuration,
			}
		} else {
			s.SpamDetails.MaxDuration = maxDuration
		}
	}
}

// WithSpammingFunc sets core function of the spammer with spamming logic, needs to use done spammer's channel to communicate.
// end of spamming and errors. Default one is the CustomConflictSpammingFunc.
func WithSpammingFunc(spammerFunc SpammingFunc) Options {
	return func(s *Spammer) {
		s.spammingFunc = spammerFunc
	}
}

// WithAccountAlias sets the alias of the account that will be used to pay with mana for sent blocks.
func WithAccountAlias(alias string) Options {
	return func(s *Spammer) {
		s.IssuerAlias = alias
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Spammer EvilWallet options ///////////////////////////////////////////////////////////////////////////////////////////////////

// WithRateSetter enables setting rate of spammer.
func WithRateSetter(enable bool) Options {
	return func(s *Spammer) {
		s.UseRateSetter = enable
	}
}

// WithEvilWallet provides evil wallet instance, that will handle all spam logic according to provided EvilScenario.
func WithEvilWallet(initWallets *evilwallet.EvilWallet) Options {
	return func(s *Spammer) {
		s.EvilWallet = initWallets
	}
}

// WithEvilScenario provides initWallet of spammer, if omitted spammer will prepare funds based on maxBlkSent parameter.
func WithEvilScenario(scenario *evilwallet.EvilScenario) Options {
	return func(s *Spammer) {
		s.EvilScenario = scenario
	}
}

func WithTimeDelayForDoubleSpend(timeDelay time.Duration) Options {
	return func(s *Spammer) {
		s.TimeDelayBetweenConflicts = timeDelay
	}
}

// WithBlowballSize provides spammer with options regarding blowball size.
func WithBlowballSize(size int) Options {
	return func(s *Spammer) {
		if s.SpamDetails == nil {
			s.SpamDetails = &SpamDetails{
				BlowballSize: size,
			}
		} else {
			s.SpamDetails.BlowballSize = size
		}
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type SpamDetails struct {
	Rate        int
	TimeUnit    time.Duration
	MaxDuration time.Duration
	// calculated based on duration, 0 for infinite spamming
	MaxBatchesSent int
	BlowballSize   int
}
