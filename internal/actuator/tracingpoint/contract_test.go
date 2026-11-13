package tracingpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestTracingPreservesRequestAttributes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		url     string
		scheme  string
		hostKey string
		host    string
		port    int64
		status  int
	}{
		{name: "HTTP hostname port", url: "http://service.example:8080/test/42", scheme: "http", hostKey: "net.host.name", host: "service.example", port: 8080, status: 200},
		{name: "HTTPS hostname port", url: "https://service.example:8443/test/42", scheme: "https", hostKey: "net.host.name", host: "service.example", port: 8443, status: 200},
		{name: "IPv4 bad request", url: "http://127.0.0.1:8080/test/42", scheme: "http", hostKey: "net.host.ip", host: "127.0.0.1", port: 8080, status: 400},
		{name: "IPv6 server error", url: "http://[::1]:8080/test/42", scheme: "http", hostKey: "net.host.ip", host: "::1", port: 8080, status: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := otel.GetTracerProvider()
			exporter := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			otel.SetTracerProvider(provider)
			t.Cleanup(func() {
				otel.SetTracerProvider(previous)
				assert.NoError(t, provider.Shutdown(context.Background()))
			})
			app := fiber.New()
			app.Use(NewOtelTracingMiddleware("contract-service", nil))
			var outgoing context.Context
			app.Get("/test/:id", func(c fiber.Ctx) error {
				request, err := http.NewRequestWithContext(c.Context(), http.MethodGet, "http://upstream", nil)
				if err != nil {
					return err
				}
				outgoing = request.Context()
				return c.SendStatus(tc.status)
			})
			request := httptest.NewRequest(http.MethodGet, tc.url, nil)
			request.Header.Set("b3", "80f198ee56343ba864fe8b2a57d3eff7-e457b5a2e4d86bd1-1")
			response, err := app.Test(request)
			require.NoError(t, err)
			defer response.Body.Close()
			assert.Equal(t, tc.status, response.StatusCode)
			require.NotNil(t, outgoing)
			assert.Equal(t, "80f198ee56343ba864fe8b2a57d3eff7", trace.SpanContextFromContext(outgoing).TraceID().String())
			spans := exporter.GetSpans()
			require.Len(t, spans, 1)
			attributes := make(map[attribute.Key]attribute.Value)
			for _, attr := range spans[0].Attributes {
				attributes[attr.Key] = attr.Value
			}
			assert.Equal(t, tc.scheme, attributes["http.scheme"].AsString())
			assert.Equal(t, tc.host, attributes[attribute.Key(tc.hostKey)].AsString())
			assert.Equal(t, tc.port, attributes["net.host.port"].AsInt64())
			assert.Equal(t, int64(tc.status), attributes["http.status_code"].AsInt64())
		})
	}
}
