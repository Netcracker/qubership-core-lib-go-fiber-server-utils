package monitorpoint

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/monitoring"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

func TestEnablePrometheus_HttpStatus200(t *testing.T) {
	updateRegister()
	app := fiber.New()
	err := EnablePrometheus("/prometheus", &monitoring.Config{}, app)
	assert.Nil(t, err)

	req := httptest.NewRequest("GET", "/prometheus", nil)
	resp, err := app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestPlatformPrometheusMiddleware_MetricsChanged(t *testing.T) {
	updateRegister()
	app := fiber.New()
	platformPrometheusMetrics, err := monitoring.RegisterPlatformPrometheusMetrics(&monitoring.Config{})
	assert.Nil(t, err)
	app.Use(PlatformPrometheusMiddleware(platformPrometheusMetrics))

	app.Get("/test", func(ctx *fiber.Ctx) error { return ctx.SendStatus(200) })
	app.Test(httptest.NewRequest("GET", "/test", nil))
	time.Sleep(100 * time.Millisecond) // wait for updating value of metrics
	labels := []string{"200", "get", "/test"}

	// check RequestStatusCounter
	values := platformPrometheusMetrics.RequestStatusCounter.WithLabelValues(labels...)
	var metric dto.Metric
	assert.Nil(t, values.Write(&metric))
	assert.Equal(t, 1., metric.GetCounter().GetValue())

	//	TODO think about RequestLatencyHistogram
}

func TestPlatformPrometheusMiddleware_UsesPathTemplate(t *testing.T) {
	updateRegister()
	app := fiber.New()
	platformPrometheusMetrics, err := monitoring.RegisterPlatformPrometheusMetrics(&monitoring.Config{})
	assert.Nil(t, err)
	app.Use(PlatformPrometheusMiddleware(platformPrometheusMetrics))

	app.Get("/test/:id", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(200)
	})

	req, err := http.NewRequest(http.MethodGet, "/test/1", nil)
	assert.Nil(t, err)

	resp, err := app.Test(req)
	assert.Nil(t, err)
	assert.NotNil(t, resp)

	time.Sleep(100 * time.Millisecond) // Wait until goroutine will finish
	counter, err := platformPrometheusMetrics.RequestStatusCounter.GetMetricWithLabelValues("200", "get", "/test/:id")
	assert.Nil(t, err)

	metric := &dto.Metric{}
	err = counter.Write(metric)
	assert.Nil(t, err)

	assert.Equal(t, float64(1), metric.Counter.GetValue())
}

func updateRegister() {
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry
}
