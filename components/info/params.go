package info

import (
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/app"
)

type ParametersInfo struct {
}

var ParamsInfo = &ParametersInfo{}
var ParamsTool = &models.ParametersTool{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"info": ParamsInfo,
		"tool": ParamsTool,
	},
	Masked: []string{
		"info",
		"app",
		"profiling",
		"logger",
	},
}
