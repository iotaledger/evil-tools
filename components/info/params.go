package info

import (
	"github.com/iotaledger/evil-tools/pkg/info"
	"github.com/iotaledger/hive.go/app"
)

var ParamsInfo = &info.ParametersInfo{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"info": ParamsInfo,
	},
	Masked: []string{
		"info",
		"app",
		"profiling",
		"logger",
	},
}
