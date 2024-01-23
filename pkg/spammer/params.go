package spammer

import "time"

type ParametersSpammer struct {
	NodeURLs  []string `default:"http://localhost:8050" usage:"API URLs for clients used in test separated with commas"`
	FaucetURL string   `default:"http://localhost:8088" usage:"Faucet URL used in test"`

	Type                  string        `default:"tx" usage:"Spammers used during test. Format: strings separated with comma, available options: 'blk' - block, 'tx' - transaction, 'ds' - double spends spammers, 'nds' - n-spends spammer, 'bb' - blowball, or one of custom scenarios that can be found in pkg/evilwallet/customscenarion.go"`
	Rate                  int           `default:"1" usage:"Spamming rate for provided 'spammer'. Format: numbers separated with comma, e.g. 10,100,1 if three spammers were provided for 'spammer' parameter."`
	Duration              time.Duration `default:"-1ns" usage:"Spam duration. If not provided spam will lats infinitely. Format: separated by commas list of decimal numbers, each with optional fraction and a unit suffix, such as '300ms', '-1.5h' or '2h45m'.\n Valid time units are 'ns', 'us', 'ms', 's', 'm', 'h'."`
	Account               string        `default:"" usage:"Account alias to be used for the spam. Account should be created first with accounts tool."`
	RateSetterEnabled     bool          `default:"false" usage:"Enable the rate setter, which will set the rate for the spammer. To enable provide an empty flag."`
	DeepSpamEnabled       bool          `default:"false" usage:"Enable the deep spam, by reusing outputs created during the spam. To enable provide an empty flag."`
	ReuseEnabled          bool          `default:"false" usage:"Enable the reuse of outputs created during the spam. To enable provide an empty flag."`
	AutoRequestingEnabled bool          `default:"false" usage:"Enable the auto-requesting, which will request tokens from faucet for the spammer. To enable provide an empty flag."`
	AutoRequestingAmount  int64         `default:"1000" usage:"Amount of tokens to be requested from faucet for the spammer. To enable provide an empty flag."`
	NSpend                int           `default:"2" usage:"Number of outputs to be spent in n-spends spammer for the spammer type needs to be set to 'ds'. Default value is 2 for double-spend."`
	BlowballSize          int           `default:"30" usage:"Size of the blowball to be used in blowball spammer. To enable provide an empty flag."`
}
