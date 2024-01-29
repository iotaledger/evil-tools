package app

import (
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/app"
)

var ParamsTool = &models.ParametersTool{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"tool": ParamsTool,
	},
	Masked: []string{
		"info",
		"app",
		"profiling",
		"logger",
	},
}
