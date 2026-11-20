package monitorpoint

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v3"
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
	return func(ctx *fiber.Ctx) error {
		begin := time.Now()
		if err := ctx.Next(); err != nil {
			return err
		}
		method := strings.ToLower(string(ctx.Context().Method()))
		code := strconv.Itoa(ctx.Response().StatusCode())
		pathTemplate := ctx.Route().Path

		go func() {
			platformPrometheusMetrics.IncRequestStatusCounter(code, method, pathTemplate)
			platformPrometheusMetrics.ObserveRequestLatencyHistogram(code, method, pathTemplate, begin)
		}()

		return nil
	}
}
