package monitorpoint

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/monitoring"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var logger logging.Logger

func init() {
	logger = logging.GetLogger("fibermntr")
}

func EnablePrometheus(url string, config *monitoring.Config, app *fiber.App) error {
	logger.Debug("Going to enable prometheus metrics")
	platformPrometheusMetrics, err := monitoring.RegisterPlatformPrometheusMetrics(config)
	if err != nil {
		logger.Errorf("Got error during prometheus metrics registry")
		return err
	}
	app.Use(PlatformPrometheusMiddleware(platformPrometheusMetrics))
	app.Get(url, adaptor.HTTPHandler(promhttp.Handler()))
	return nil
}

func PlatformPrometheusMiddleware(platformPrometheusMetrics *monitoring.PlatformPrometheusMetrics) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		if refusedBeforeRouting(ctx) {
			return ctx.Next()
		}
		begin := time.Now()
		if err := ctx.Next(); err != nil {
			return err
		}
		method := strings.ToLower(string(ctx.RequestCtx().Method()))
		code := strconv.Itoa(ctx.Response().StatusCode())
		pathTemplate := ctx.Route().Path

		go func() {
			platformPrometheusMetrics.IncRequestStatusCounter(code, method, pathTemplate)
			platformPrometheusMetrics.ObserveRequestLatencyHistogram(code, method, pathTemplate, begin)
		}()

		return nil
	}
}

// refusedBeforeRouting reports whether the server refused the request while reading it: a body over BodyLimit,
// headers over ReadBufferSize, or a malformed Content-Length. Fiber v3 sets the error status before it runs the
// middleware for such a request, so a status of 400 or above on entry marks it. Such a request has no route to name a
// span or label a metric with, and Fiber v2 did not run the middleware for it at all.
func refusedBeforeRouting(c fiber.Ctx) bool {
	return c.Response().StatusCode() >= fiber.StatusBadRequest
}
