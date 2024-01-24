package accounts

import (
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/app"
)

type (
	ParametersAccountsCreate struct {
		Alias                string `default:"" usage:"Alias name of the account to create"`
		NoBlockIssuerFeature bool   `default:"false" usage:"Create account without Block Issuer Feature, can only be set false no if implicit is false, as each account created implicitly needs to have BIF."`
		Implicit             bool   `default:"false" usage:"Create an implicit account"`
		Transition           bool   `default:"false" usage:"account should be transitioned to a full account if created with implicit address. Transition disabled by default, to enable provide an empty flag."`
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

	ParametersRewards struct {
		Alias string `default:"" usage:"The alias name of the wallet to get rewards for"`
	}

	ParametersAccountsDelegate struct {
		FromAlias string `default:"" usage:"The alias of the account to delegate IOTA tokens from"`
		ToAddress string `default:"rms1pzg8cqhfxqhq7pt37y8cs4v5u4kcc48lquy2k73ehsdhf5ukhya3y5rx2w6" usage:"The account address of the account to delegate IOTA tokens to"`
		Amount    int64  `default:"100" usage:"The amount of mana to delegate"`
		CheckPool bool   `default:"false" usage:"Check if the delegation is added to pool stake when the start epoch is committed"`
	}

	ParametersAccountsUpdate struct {
		Alias          string `default:"" usage:"Alias name of the account to update"`
		BlockIssuerKey string `default:"" usage:"Block issuer key (in hex) to add"`
		Amount         int64  `default:"100" usage:"Amount of token to add"`
		Mana           int64  `default:"100" usage:"Amount of mana to add"`
		ExpirySlot     int64  `default:"0" usage:"Update the expiry slot of the account"`
	}

	ParameterAccountsInfo struct {
		Alias   string `default:"" usage:"Alias name of the account to get info"`
		Verbose bool   `default:"false" usage:"Verbose output"`
	}

	ParametersAccounts struct {
		Create   ParametersAccountsCreate
		Convert  ParametersAccountsConvert
		Destroy  ParametersAccountsDestroy
		Allot    ParametersAccountsAllot
		Stake    ParametersAccountsStake
		Rewards  ParametersRewards
		Delegate ParametersAccountsDelegate
		Update   ParametersAccountsUpdate
		Info     ParameterAccountsInfo
	}
)

var ParamsAccounts = &ParametersAccounts{}
var ParamsTool = &models.ParametersTool{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"accounts": ParamsAccounts,
		"tool":     ParamsTool,
	},
	Masked: []string{
		"profiling",
		"logger",
		"app",
	},
}
