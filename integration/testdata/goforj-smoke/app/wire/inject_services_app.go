package wire

import (
	"example.com/feature-release-smoke/app"
	"example.com/feature-release-smoke/internal/flagdemo"
	"example.com/feature-release-smoke/internal/runtime"
	"github.com/goforj/wire"
)

// appSet injects the native owner, facade, and application service through app-owned providers.
var appSet = wire.NewSet(app.NewLifecycleRegistry, runtime.NewTimeouts, flagdemo.OpenEngine, flagdemo.NewFeatures, flagdemo.NewService)
