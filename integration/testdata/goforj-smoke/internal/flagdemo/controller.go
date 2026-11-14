package flagdemo

import (
	"net/http"

	"github.com/goforj/feature"
	"github.com/goforj/web"
)

// Controller exposes only fake smoke decisions and classified source health on loopback.
type Controller struct {
	service *Service
	engine  *feature.Client
}

// NewController receives the app-owned read service and native health source through Wire.
func NewController(service *Service, engine *feature.Client) *Controller {
	return &Controller{service: service, engine: engine}
}

// Routes registers isolated smoke endpoints under the application's API route group.
func (c *Controller) Routes() []web.Route {
	return []web.Route{
		web.NewRoute(http.MethodGet, "/feature-smoke", c.Decide),
		web.NewRoute(http.MethodGet, "/feature-smoke/status", c.Status),
		web.NewRoute(http.MethodPost, "/feature-smoke/release", c.Release),
	}
}

// Decide exercises a real HTTP request through the injected application service.
func (c *Controller) Decide(r web.Context) error {
	result, err := c.service.Decide(r.Context(), r.Query("hold") == "true")
	if err != nil {
		return err
	}
	return r.JSON(http.StatusOK, result)
}

// Status reports safe health and request coordination without source paths or targeting data.
func (c *Controller) Status(r web.Context) error {
	status := c.engine.Status()
	classification := ""
	if status.LastError != nil {
		classification = status.LastError.Error()
	}
	return r.JSON(http.StatusOK, map[string]any{"revision": status.Revision, "error": classification, "captured": c.service.captured.Load()})
}

// Release opens the request gate used to prove snapshot consistency across publication.
func (c *Controller) Release(r web.Context) error {
	c.service.Release()
	return r.JSON(http.StatusOK, map[string]bool{"released": true})
}
