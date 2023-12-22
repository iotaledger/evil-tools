package spammer

import (
	"time"

	"github.com/iotaledger/evil-tools/pkg/evilwallet"
	"github.com/iotaledger/hive.go/runtime/options"
)

// WithRate provides spammer with options regarding rate, time unit, and finishing spam criteria. Provide 0 to one of max parameters to skip it.
func WithRate(rate int) options.Option[Spammer] {
	return func(s *Spammer) {
		s.Rate = rate
	}
}

// WithSpamDuration provides spammer with options regarding rate, time unit, and finishing spam criteria. Provide 0 to one of max parameters to skip it.
func WithSpamDuration(maxDuration time.Duration) options.Option[Spammer] {
	return func(s *Spammer) {
		s.MaxDuration = maxDuration
	}
}

// WithSpammingFunc sets core function of the spammer with spamming logic, needs to use done spammer's channel to communicate.
// end of spamming and errors. Default one is the CustomConflictSpammingFunc.
func WithSpammingFunc(spammerFunc SpammingFunc) options.Option[Spammer] {
	return func(s *Spammer) {
		s.spammingFunc = spammerFunc
	}
}

// WithAccountAlias sets the alias of the account that will be used to pay with mana for sent blocks.
func WithAccountAlias(alias string) options.Option[Spammer] {
	return func(s *Spammer) {
		s.IssuerAlias = alias
	}
}

// WithRateSetter enables setting rate of spammer.
func WithRateSetter(enable bool) options.Option[Spammer] {
	return func(s *Spammer) {
		s.UseRateSetter = enable
	}
}

// WithEvilWallet provides evil wallet instance, that will handle all spam logic according to provided EvilScenario.
func WithEvilWallet(initWallets *evilwallet.EvilWallet) options.Option[Spammer] {
	return func(s *Spammer) {
		s.EvilWallet = initWallets
	}
}

// WithEvilScenario provides initWallet of spammer, if omitted spammer will prepare funds based on maxBlkSent parameter.
func WithEvilScenario(scenario *evilwallet.EvilScenario) options.Option[Spammer] {
	return func(s *Spammer) {
		s.EvilScenario = scenario
	}
}

// WithBlowballSize provides spammer with options regarding blowball size.
func WithBlowballSize(size int) options.Option[Spammer] {
	return func(s *Spammer) {
		s.BlowballSize = size
	}
}
