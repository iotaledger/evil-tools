package eviltools

import (
	"github.com/iotaledger/evil-tools/pkg/accountwallet"
	"github.com/iotaledger/evil-tools/pkg/spammer"
	"github.com/iotaledger/hive.go/app"
)

type ParametersEvilTools struct {
	NodeURLs  []string `default:"http://localhost:8050" usage:"API URLs for clients used in test separated with commas"`
	FaucetURL string   `default:"http://localhost:8088" usage:"Faucet URL used in test"`

	Spammer  spammer.ParametersSpammer
	Accounts accountwallet.ParametersAccounts
}

var ParamsEvilTools = &ParametersEvilTools{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"eviltools": ParamsEvilTools,
	},
}
