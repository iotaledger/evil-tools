package spammer

import (
	"github.com/iotaledger/evil-tools/pkg/spammer"
	"github.com/iotaledger/hive.go/app"
)

var ParamsSpammer = &spammer.ParametersSpammer{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"spammer": ParamsSpammer,
	},
	Masked: []string{
		"profiling",
		"logger",
		"app",
	},
}
