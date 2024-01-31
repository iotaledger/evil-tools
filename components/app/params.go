package app

import (
	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/hive.go/app"
)

var params = &app.ComponentParams{
	Params: map[string]any{
		"tool": models.ParamsTool,
	},
	Masked: []string{
		"tool.blockissuerprivatekey",
		"info",
		"app",
		"profiling",
		"logger",
	},
}
