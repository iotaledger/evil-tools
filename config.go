package main

import (
	"time"

	"github.com/iotaledger/evil-tools/accountwallet"
	"github.com/iotaledger/evil-tools/evilwallet"
	"github.com/iotaledger/evil-tools/programs"
	"github.com/iotaledger/evil-tools/spammer"
)

// Nodes used during the test, use at least two nodes to be able to double spend.
var (
	// urls = []string{"http://bootstrap-01.feature.shimmer.iota.cafe:8080", "http://vanilla-01.feature.shimmer.iota.cafe:8080", "http://drng-01.feature.shimmer.iota.cafe:8080"}
	urls = []string{"http://localhost:8050", "http://localhost:8060"} //, "http://localhost:8070", "http://localhost:8040"}
)

var (
	Script = ScriptSpammer

	customSpamParams = programs.CustomSpamParams{
		ClientURLs:            urls,
		FaucetURL:             "http://localhost:8088",
		SpamType:              spammer.TypeBlock,
		Rate:                  1,
		TimeUnit:              time.Second,
		DelayBetweenConflicts: 0,
		NSpend:                2,
		Scenario:              evilwallet.Scenario1(),
		ScenarioName:          "guava",
		DeepSpam:              false,
		EnableRateSetter:      false,
		AccountAlias:          accountwallet.FaucetAccountAlias,
		BlowballSize:          30,
	}

	accountsSubcommandsFlags []accountwallet.AccountSubcommands
)
