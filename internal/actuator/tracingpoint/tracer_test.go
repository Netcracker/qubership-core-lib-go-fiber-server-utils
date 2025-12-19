package tracingpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/tracing"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	zkmodel "github.com/openzipkin/zipkin-go/model"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func tracingHandler(t *testing.T) (http.HandlerFunc, *bool) {
	gotRequest := false
	var handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequest = true
		var spans []zkmodel.SpanModel
		if err := json.NewDecoder(r.Body).Decode(&spans); err != nil {
			t.Error(err)
		}
		span := spans[0]
		assert.Equal(t, "service-name-namespace", span.LocalEndpoint.ServiceName)
		assert.Equal(t, "/test", span.Tags["http.target"])
		w.WriteHeader(fiber.StatusOK)
	})
	return handler, &gotRequest
}

func flashBatches(t *testing.T) {
	tp := otel.GetTracerProvider()
	tracerErr := tp.(*sdktrace.TracerProvider).Shutdown(context.Background())
	if tracerErr != nil {
		t.Fatalf("Failed to shutdown tracer provider: %v", tracerErr)
	}
}

func TestEnableOtelTracingZipkin_WithDefault(t *testing.T) {
	handler, gotRequest := tracingHandler(t)
	server, tracerHost := createTestServer(handler)
	defer server.Close()

	os.Setenv("tracing.enabled", "true")
	os.Setenv("tracing.host", tracerHost)
	os.Setenv("microservice.name", "service-name")
	os.Setenv("microservice.namespace", "namespace")
	defer func() {
		os.Unsetenv("tracing.enabled")
		os.Unsetenv("tracing.host")
		os.Unsetenv("microservice.name")
		os.Unsetenv("microservice.namespace")
	}()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})

	app := fiber.New()
	err := EnableOtelTracing(tracing.NewZipkinTracer(), app)
	assert.Nil(t, err)
	app.Get("/test", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})
	_, err = app.Test(httptest.NewRequest("GET", "/test", nil))
	flashBatches(t)
	assert.Nil(t, err)
	assert.True(t, *gotRequest)
}

func TestEnableOtelTracingZipkin_WithOptions(t *testing.T) {
	handler, gotRequest := tracingHandler(t)
	server, tracerHost := createTestServer(handler)
	defer server.Close()

	app := fiber.New()
	options := createDefaultZipkinOptions(tracerHost)
	err := EnableOtelTracing(tracing.NewZipkinTracerWithOpts(options), app)
	assert.Nil(t, err)
	app.Get("/test", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})
	_, err = app.Test(httptest.NewRequest("GET", "/test", nil))
	flashBatches(t)
	assert.Nil(t, err)
	assert.True(t, *gotRequest)
}

func TestEnableOtelTracingZipkin_B3_WithoutHeader(t *testing.T) {
	handler, gotRequest := tracingHandler(t)
	server, tracerHost := createTestServer(handler)
	defer server.Close()

	app := fiber.New()
	options := createDefaultZipkinOptions(tracerHost)
	err := EnableOtelTracing(tracing.NewZipkinTracerWithOpts(options), app)
	assert.Nil(t, err)

	app.Get("/test", func(ctx *fiber.Ctx) error {
		uCtx := ctx.UserContext()
		sc := trace.SpanContextFromContext(uCtx)
		assert.True(t, sc.IsValid()) // must contain random traceId and spanId
		return ctx.SendStatus(fiber.StatusOK)
	})

	testRequest := httptest.NewRequest(http.MethodGet, "http://localhost:10000/test", nil)
	resp, err := app.Test(testRequest, 3000)

	flashBatches(t)
	assert.Nil(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.True(t, *gotRequest)
}

func TestEnableOtelTracingZipkin_B3_WithSingleHeader(t *testing.T) {
	handler, gotRequest := tracingHandler(t)
	server, tracerHost := createTestServer(handler)
	defer server.Close()

	app := fiber.New()
	options := createDefaultZipkinOptions(tracerHost)
	err := EnableOtelTracing(tracing.NewZipkinTracerWithOpts(options), app)
	assert.Nil(t, err)

	b3TraceIdValue := "80f198ee56343ba864fe8b2a57d3eff7"
	b3SpanIdValue := "e457b5a2e4d86bd1"
	b3SampledValue := "1"
	b3ParentSpanIdValue := "05e3ac9a4f6e3b90"
	b3Value := fmt.Sprintf("%s-%s-%s-%s", b3TraceIdValue, b3SpanIdValue, b3SampledValue, b3ParentSpanIdValue)

	app.Get("/test", func(ctx *fiber.Ctx) error {
		uCtx := ctx.UserContext()
		sc := trace.SpanContextFromContext(uCtx)
		assert.Equal(t, b3TraceIdValue, sc.TraceID().String())
		assert.NotEqual(t, b3SpanIdValue, sc.SpanID().String())
		return ctx.SendStatus(fiber.StatusOK)
	})

	testRequest, _ := http.NewRequest(http.MethodGet, "http://localhost:10000/test", nil)
	testRequest.Header.Set("b3", b3Value)
	resp, err := app.Test(testRequest, 3000)
	flashBatches(t)

	assert.Nil(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.True(t, *gotRequest)
}

func TestEnableOtelTracingZipkin_B3_WithSingleHeader_BadTraceId(t *testing.T) {
	handler, gotRequest := tracingHandler(t)
	server, tracerHost := createTestServer(handler)
	defer server.Close()

	app := fiber.New()
	options := createDefaultZipkinOptions(tracerHost)
	err := EnableOtelTracing(tracing.NewZipkinTracerWithOpts(options), app)
	assert.Nil(t, err)

	b3TraceIdValue := "80f198ee56343ba864fe8b2a5mangled"
	b3SpanIdValue := "e457b5a2e4d86bd1"
	b3SampledValue := "1"
	b3ParentSpanIdValue := "05e3ac9a4f6e3b90"
	b3Value := fmt.Sprintf("%s-%s-%s-%s", b3TraceIdValue, b3SpanIdValue, b3SampledValue, b3ParentSpanIdValue)

	app.Get("/test", func(ctx *fiber.Ctx) error {
		uCtx := ctx.UserContext()
		sc := trace.SpanContextFromContext(uCtx)
		assert.True(t, sc.IsValid())
		assert.NotEqual(t, b3TraceIdValue, sc.TraceID().String())
		assert.NotEqual(t, b3SpanIdValue, sc.SpanID().String())
		return ctx.SendStatus(fiber.StatusOK)
	})

	testRequest, _ := http.NewRequest(http.MethodGet, "http://localhost:10000/test", nil)
	testRequest.Header.Set("b3", b3Value)
	resp, err := app.Test(testRequest, 3000)
	flashBatches(t)

	assert.Nil(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.True(t, *gotRequest)
}

func TestEnableOtelTracingZipkin_B3_WithMultipleHeaders(t *testing.T) {
	handler, gotRequest := tracingHandler(t)
	server, tracerHost := createTestServer(handler)
	defer server.Close()

	app := fiber.New()
	options := createDefaultZipkinOptions(tracerHost)
	err := EnableOtelTracing(tracing.NewZipkinTracerWithOpts(options), app)
	assert.Nil(t, err)

	b3TraceIdValue := "80f198ee56343ba864fe8b2a57d3eff7"
	b3SpanIdValue := "e457b5a2e4d86bd1"

	app.Get("/test", func(ctx *fiber.Ctx) error {
		uCtx := ctx.UserContext()
		sc := trace.SpanContextFromContext(uCtx)
		assert.Equal(t, b3TraceIdValue, sc.TraceID().String())
		assert.NotEqual(t, b3SpanIdValue, sc.SpanID().String())
		return ctx.SendStatus(fiber.StatusOK)
	})

	testRequest, _ := http.NewRequest(http.MethodGet, "http://localhost:10000/test", nil)
	testRequest.Header.Set("X-B3-TraceId", b3TraceIdValue)
	testRequest.Header.Set("X-B3-SpanId", b3SpanIdValue)
	resp, err := app.Test(testRequest, 3000)
	flashBatches(t)

	assert.Nil(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.True(t, *gotRequest)
}

func TestEnableOtelTracingZipkin_B3_WithMultipleHeaders_BadTraceId(t *testing.T) {
	handler, gotRequest := tracingHandler(t)
	server, tracerHost := createTestServer(handler)
	defer server.Close()

	app := fiber.New()
	options := createDefaultZipkinOptions(tracerHost)
	err := EnableOtelTracing(tracing.NewZipkinTracerWithOpts(options), app)
	assert.Nil(t, err)

	b3TraceIdValue := "80f198ee56343ba"
	b3SpanIdValue := "e457b5a2e4d86bd1"

	app.Get("/test", func(ctx *fiber.Ctx) error {
		uCtx := ctx.UserContext()
		sc := trace.SpanContextFromContext(uCtx)
		assert.True(t, sc.IsValid())
		assert.NotEqual(t, b3TraceIdValue, sc.TraceID().String())
		assert.NotEqual(t, b3SpanIdValue, sc.SpanID().String())
		return ctx.SendStatus(fiber.StatusOK)
	})

	testRequest, err := http.NewRequest(http.MethodGet, "http://localhost:10000/test", nil)
	assert.Nil(t, err)

	testRequest.Header.Set("X-B3-TraceId", b3TraceIdValue)
	testRequest.Header.Set("X-B3-SpanId", b3SpanIdValue)
	resp, err := app.Test(testRequest, 3000)
	flashBatches(t)

	assert.Nil(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.True(t, *gotRequest)
}

func TestEnableOtelTracingZipkin_B3_WithMultipleHeaders_TestCaseInsensitivity(t *testing.T) {
	handler, gotRequest := tracingHandler(t)
	server, tracerHost := createTestServer(handler)
	defer server.Close()

	app := fiber.New()
	options := createDefaultZipkinOptions(tracerHost)
	err := EnableOtelTracing(tracing.NewZipkinTracerWithOpts(options), app)
	assert.Nil(t, err)

	b3TraceIdValue := "80f198ee56343ba864fe8b2a57d3eff7"
	b3SpanIdValue := "e457b5a2e4d86bd1"

	app.Get("/test", func(ctx *fiber.Ctx) error {
		uCtx := ctx.UserContext()
		sc := trace.SpanContextFromContext(uCtx)
		assert.Equal(t, b3TraceIdValue, sc.TraceID().String())
		assert.NotEqual(t, b3SpanIdValue, sc.SpanID().String())
		return ctx.SendStatus(fiber.StatusOK)
	})

	testRequest, _ := http.NewRequest(http.MethodGet, "http://localhost:10000/test", nil)
	testRequest.Header.Set("X-B3-TRACEID", b3TraceIdValue)
	testRequest.Header.Set("X-B3-SPANID", b3SpanIdValue)
	resp, err := app.Test(testRequest, 3000)
	assert.Nil(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	testRequest, _ = http.NewRequest(http.MethodGet, "http://localhost:10000/test", nil)
	testRequest.Header.Set("x-b3-traceid", b3TraceIdValue)
	testRequest.Header.Set("x-b3-spanid", b3SpanIdValue)
	resp, err = app.Test(testRequest, 3000)
	flashBatches(t)

	assert.Nil(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.True(t, *gotRequest)
}

func createTestServer(handler http.HandlerFunc) (*httptest.Server, string) {
	server := httptest.NewUnstartedServer(handler)
	tracerHost := "127.0.0.1"
	tracerPort := ":9411"
	l, _ := net.Listen("tcp", tracerHost+tracerPort)
	server.Listener = l
	server.Start()
	return server, tracerHost
}

func createDefaultZipkinOptions(tracerHost string) tracing.ZipkinOptions {
	return tracing.ZipkinOptions{
		TracingEnabled:             true,
		TracingHost:                tracerHost,
		TracingSamplerRateLimiting: 10,
		ServiceName:                "service-name",
		Namespace:                  "namespace",
	}
}