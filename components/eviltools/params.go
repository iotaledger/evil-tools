package eviltools

import (
	"github.com/iotaledger/evil-tools/pkg/spammer"
	"github.com/iotaledger/hive.go/app"
)

type (
	ParametersAccountsCreate struct {
		Alias                string `default:"" usage:"Alias name of the account to update"`
		NoBlockIssuerFeature bool   `default:"false" usage:"Create account without Block Issuer Feature, can only be set false no if implicit is false, as each account created implicitly needs to have BIF."`
		Implicit             bool   `default:"false" usage:"Create an implicit account"`
		NoTransition         bool   `default:"false" usage:"account should not be transitioned to a full account if created with implicit address. Transition enabled by default, to disable provide an empty flag."`
	}

	ParametersAccountsConvert struct {
		Alias string `default:"" usage:"The implicit account to be converted to full account with BIF"`
	}

	ParametersAccountsDestroy struct {
		Alias      string `default:"" usage:"The alias name of the account to be destroyed"`
		ExpirySlot int64  `default:"0" usage:"The expiry slot of the account to be destroyed"`
	}

	ParametersAccountsAllot struct {
		AllotToAccount string `default:"" usage:"The alias name of the account to allot mana to"`
		Amount         int64  `default:"1000" usage:"The amount of mana to allot"`
	}

	ParametersAccountsStake struct {
		Alias      string `default:"" usage:"The alias name of the account to stake"`
		Amount     int64  `default:"100" usage:"The amount of tokens to stake"`
		FixedCost  int64  `default:"0" usage:"The fixed cost of the account to stake"`
		StartEpoch int64  `default:"0" usage:"The start epoch of the account to stake"`
		EndEpoch   int64  `default:"0" usage:"The end epoch of the account to stake"`
	}

	ParametersAccountsDelegate struct {
		FromAccount string `default:"" usage:"The alias name of the account to delegate mana from"`
		ToAccount   string `default:"" usage:"The alias of the account to delegate mana to"`
		Amount      int64  `default:"100" usage:"The amount of mana to delegate"`
	}

	ParametersAccountsUpdate struct {
		Alias          string `default:"" usage:"Alias name of the account to update"`
		BlockIssuerKey string `default:"" usage:"Block issuer key (in hex) to add"`
		Amount         int64  `default:"100" usage:"Amount of token to add"`
		Mana           int64  `default:"100" usage:"Amount of mana to add"`
		ExpirySlot     int64  `default:"0" usage:"Update the expiry slot of the account"`
	}

	ParametersAccounts struct {
		AccountStatesFile     string `default:"wallet.dat" usage:"File to store account states in"`
		BlockIssuerPrivateKey string `default:"db39d2fde6301d313b108dc9db1ee724d0f405f6fde966bd776365bc5f4a5fb31e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648" usage:"Block issuer private key (in hex) to use for genesis account"`
		AccountID             string `default:"0x6aee704f25558e8aa7630fed0121da53074188abc423b3c5810f80be4936eb6e" usage:"Account ID to use for genesis account"`

		Create   ParametersAccountsCreate
		Convert  ParametersAccountsConvert
		Destroy  ParametersAccountsDestroy
		Allot    ParametersAccountsAllot
		Stake    ParametersAccountsStake
		Delegate ParametersAccountsDelegate
		Update   ParametersAccountsUpdate
	}
)

type ParametersEvilTools struct {
	NodeURLs  []string `default:"http://localhost:8050" usage:"API URLs for clients used in test separated with commas"`
	FaucetURL string   `default:"http://localhost:8088" usage:"Faucet URL used in test"`

	Spammer  spammer.ParametersSpammer
	Accounts ParametersAccounts
}

var ParamsEvilTools = &ParametersEvilTools{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"eviltools": ParamsEvilTools,
	},
}
