package app

import (
	"example.com/feature-release-smoke/internal/flagdemo"
	"example.com/feature-release-smoke/internal/hello"
	"github.com/goforj/web"
)

// ProvideRoutes combines generated hello routes with an app-owned feature decision controller.
func ProvideRoutes(helloController *hello.Controller, flags *flagdemo.Controller) []web.RouteGroup {
	return []web.RouteGroup{web.NewRouteGroup("/api/v1", append(helloController.Routes(), flags.Routes()...))}
}
