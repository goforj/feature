package wire

import (
	"example.com/feature-release-smoke/internal/flagdemo"
	"example.com/feature-release-smoke/internal/hello"
	"github.com/goforj/wire"
)

// appHttpControllerSet registers generated and app-owned HTTP controllers.
var appHttpControllerSet = wire.NewSet(hello.NewController, flagdemo.NewController)
