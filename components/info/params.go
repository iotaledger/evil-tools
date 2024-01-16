package info

import (
	"github.com/iotaledger/hive.go/app"
)

type ParametersInfo struct {
	NodeURLs []string `default:"http://localhost:8050" usage:"API URLs for clients used in test separated with commas"`
}

var ParamsInfo = &ParametersInfo{}

var params = &app.ComponentParams{
	Params: map[string]any{
		"info": ParamsInfo,
	},
	Masked: []string{
		"app",
		"profiling",
		"logger",
	},
}
