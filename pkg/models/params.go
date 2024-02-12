package models

var ParamsTool = &ParametersTool{}

type ParametersTool struct {
	NodeURLs  []string `default:"http://localhost:8050" usage:"API URLs for clients used in test separated with commas"`
	FaucetURL string   `default:"http://localhost:8088" usage:"Faucet URL used in test"`

	AccountStatesFile                   string `default:"wallet.dat" usage:"File to store account states in"`
	BlockIssuerPrivateKey               string `default:"db39d2fde6301d313b108dc9db1ee724d0f405f6fde966bd776365bc5f4a5fb31e4b21eb51dcddf65c20db1065e1f1514658b23a3ddbf48d30c0efc926a9a648" usage:"Block issuer private key (in hex) to use for genesis account spams"`
	AccountID                           string `default:"0x6aee704f25558e8aa7630fed0121da53074188abc423b3c5810f80be4936eb6e" usage:"Account ID to use for genesis account"`
	FaucetRequestsBlockIssuerPrivateKey string `default:"" usage:"Block issuer private key (in hex) to use for funds preparation from faucet outputs"`
	FaucetRequestsAccountID             string `default:"" usage:"Account ID to use for fund preparation."`
}
