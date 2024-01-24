package spammer

import (
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/spammer"
	"github.com/iotaledger/hive.go/app"
)

var ParamsSpammer = &spammer.ParametersSpammer{}
var ParamsTool = &models.ParametersTool{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"spammer": ParamsSpammer,
		"tool":    ParamsTool,
	},

	Masked: []string{
		"profiling",
		"logger",
		"app",
	},
}
