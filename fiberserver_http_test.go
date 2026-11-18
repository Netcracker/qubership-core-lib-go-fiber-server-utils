package fiberserver

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/apiversion"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/health"
	errs "github.com/netcracker/qubership-core-lib-go-error-handling/v3/errors"
	"github.com/netcracker/qubership-core-lib-go-error-handling/v3/tmf"
	serverErrors "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3/errors"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

var unknownErrorCode = errs.ErrorCode{Code: "TEST-0001", Title: "unexpected test error"}

type tmfFields struct {
	Code    string
	Message string
	Status  string
}

type httpResult struct {
	status     int
	header     http.Header
	body       string
	clientPort int
}

func TestRoutingErrorsForUnmatchedRequests(t *testing.T) {
	tmfErrorHandler := fiber.Config{ErrorHandler: serverErrors.DefaultErrorHandler(unknownErrorCode)}
	fiberErrorHandler := fiber.Config{}

	tests := []struct {
		name            string
		config          fiber.Config
		method          string
		target          string
		wantStatus      int
		wantContentType string
		wantAllow       string
		wantText        string
		wantTMF         *tmfFields
	}{
		{name: "TMF handler: HEAD on a GET route is served", config: tmfErrorHandler, method: "HEAD", target: "/items",
			wantStatus: 200, wantContentType: "text/plain; charset=utf-8", wantText: ""},
		{name: "TMF handler: OPTIONS on a GET route is 405", config: tmfErrorHandler, method: "OPTIONS", target: "/items",
			wantStatus: 405, wantContentType: "application/json", wantAllow: "GET, HEAD",
			wantTMF: &tmfFields{Code: "TEST-0001", Message: "Method Not Allowed", Status: "405"}},
		{name: "TMF handler: POST on a GET route is 405", config: tmfErrorHandler, method: "POST", target: "/items",
			wantStatus: 405, wantContentType: "application/json", wantAllow: "GET, HEAD",
			wantTMF: &tmfFields{Code: "TEST-0001", Message: "Method Not Allowed", Status: "405"}},
		{name: "TMF handler: unknown method is 501 in plain text", config: tmfErrorHandler, method: "FOO", target: "/items",
			wantStatus: 501, wantContentType: "text/plain; charset=utf-8", wantText: "Not Implemented"},
		{name: "TMF handler: QUERY on a GET route is 405", config: tmfErrorHandler, method: "QUERY", target: "/items",
			wantStatus: 405, wantContentType: "application/json", wantAllow: "GET, HEAD",
			wantTMF: &tmfFields{Code: "TEST-0001", Message: "Method Not Allowed", Status: "405"}},
		{name: "TMF handler: unknown path is 404", config: tmfErrorHandler, method: "GET", target: "/missing",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: &tmfFields{Code: "TEST-0001", Message: "Not Found", Status: "404"}},
		{name: "TMF handler: HEAD on an unknown path is 404 without a body", config: tmfErrorHandler, method: "HEAD", target: "/missing",
			wantStatus: 404, wantContentType: "application/json", wantText: ""},
		{name: "Fiber handler: HEAD on a GET route is served", config: fiberErrorHandler, method: "HEAD", target: "/items",
			wantStatus: 200, wantContentType: "text/plain; charset=utf-8", wantText: ""},
		{name: "Fiber handler: OPTIONS on a GET route is 405", config: fiberErrorHandler, method: "OPTIONS", target: "/items",
			wantStatus: 405, wantContentType: "text/plain; charset=utf-8", wantAllow: "GET, HEAD", wantText: "Method Not Allowed"},
		{name: "Fiber handler: POST on a GET route is 405", config: fiberErrorHandler, method: "POST", target: "/items",
			wantStatus: 405, wantContentType: "text/plain; charset=utf-8", wantAllow: "GET, HEAD", wantText: "Method Not Allowed"},
		{name: "Fiber handler: unknown method is 501", config: fiberErrorHandler, method: "FOO", target: "/items",
			wantStatus: 501, wantContentType: "text/plain; charset=utf-8", wantText: "Not Implemented"},
		{name: "Fiber handler: QUERY on a GET route is 405", config: fiberErrorHandler, method: "QUERY", target: "/items",
			wantStatus: 405, wantContentType: "text/plain; charset=utf-8", wantAllow: "GET, HEAD", wantText: "Method Not Allowed"},
		{name: "Fiber handler: unknown path is 404", config: fiberErrorHandler, method: "GET", target: "/missing",
			wantStatus: 404, wantContentType: "text/plain; charset=utf-8", wantText: "Not Found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareBuilderEnvironment(t)
			app, err := New(tt.config).Process()
			require.NoError(t, err)
			app.Get("/items", func(c fiber.Ctx) error {
				return c.SendString("ok")
			})
			addr := serve(t, app)

			result := send(t, addr, rawRequest(tt.method, tt.target, addr))

			assert.Equal(t, tt.wantStatus, result.status)
			assert.Equal(t, tt.wantContentType, result.header.Get("Content-Type"))
			assert.Equal(t, tt.wantAllow, result.header.Get("Allow"))
			if tt.wantTMF != nil {
				assert.Equal(t, *tt.wantTMF, decodeTMF(t, result.body))
			} else {
				assert.Equal(t, tt.wantText, result.body)
			}
		})
	}
}

func TestRequestContextFromIncomingHeaders(t *testing.T) {
	prepareBuilderEnvironment(t)
	app, err := New().Process()
	require.NoError(t, err)
	app.Get("/context", func(c fiber.Ctx) error {
		ctx := c.Context()
		return c.SendString(getRequestId(ctx) + " " + getTenantId(ctx) + " " + getXChannelRequestId(ctx))
	})
	addr := serve(t, app)

	result := send(t, addr, rawRequest("GET", "/context", addr,
		"X-Request-Id: request-1", "Tenant: tenant-1", "X-Channel-Request-Id: channel-1"))

	assert.Equal(t, 200, result.status)
	assert.Equal(t, "request-1 tenant-1 channel-1", result.body, "context values seen by the handler")
	assert.Equal(t, "request-1", result.header.Get("X-Request-Id"))
	assert.Equal(t, "channel-1", result.header.Get("X-Channel-Request-Id"))
	assert.Equal(t, "", result.header.Get("Tenant"))
}

func TestActuatorEndpointsResponseFormat(t *testing.T) {
	prepareBuilderEnvironment(t)
	usePrometheusRegistry(t)
	healthService, err := health.NewHealthService()
	require.NoError(t, err)
	apiVersionService, err := apiversion.NewApiVersionService(apiversion.ApiVersionConfig{PathToApiVersionInfoFile: "./testdata/api-version-info.json"})
	require.NoError(t, err)
	app, err := New().
		WithHealth("/health", healthService).
		WithApiVersion(apiVersionService).
		WithLogLevelsInfo().
		WithPrometheus("/prometheus").
		Process()
	require.NoError(t, err)
	addr := serve(t, app)

	tests := []struct {
		target          string
		wantContentType string
	}{
		{target: "/health", wantContentType: "application/json"},
		{target: "/api-version", wantContentType: "application/json"},
		{target: "/api/logging/v1/levels", wantContentType: "application/json"},
		{target: "/prometheus", wantContentType: "text/plain; version=0.0.4; charset=utf-8; escaping=underscores"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			result := send(t, addr, rawRequest("GET", tt.target, addr))

			assert.Equal(t, 200, result.status)
			assert.Equal(t, tt.wantContentType, result.header.Get("Content-Type"))
		})
	}
	t.Run("health body", func(t *testing.T) {
		result := send(t, addr, rawRequest("GET", "/health", addr))

		assert.JSONEq(t, `{"status":"UP"}`, result.body)
	})
}

func TestDeprecatedApiBlocking(t *testing.T) {
	addr := serveDeprecatedApiApp(t)
	declined := func(method, uri, methods, pattern string) *tmfFields {
		return &tmfFields{
			Code: "NC-COMMON-2101",
			Message: "Request [" + method + "] '" + uri + "' is declined with 404 Not Found, " +
				"because the following deprecated REST API is disabled: [" + methods + "] " + pattern,
			Status: "404",
		}
	}

	tests := []struct {
		name            string
		method          string
		target          string
		wantStatus      int
		wantContentType string
		wantText        string
		wantTMF         *tmfFields
	}{
		{name: "GET listed for the pattern is declined", method: "GET", target: "/deprecated-api/v1/items",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("GET", "/deprecated-api/v1/items", "[GET PUT DELETE]", "/deprecated-api/v1/**")},
		{name: "PUT listed for the pattern is declined", method: "PUT", target: "/deprecated-api/v1/items",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("PUT", "/deprecated-api/v1/items", "[GET PUT DELETE]", "/deprecated-api/v1/**")},
		{name: "DELETE listed for the pattern is declined", method: "DELETE", target: "/deprecated-api/v1/items",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("DELETE", "/deprecated-api/v1/items", "[GET PUT DELETE]", "/deprecated-api/v1/**")},
		{name: "POST not listed for the pattern is served", method: "POST", target: "/deprecated-api/v1/items",
			wantStatus: 200, wantContentType: "text/plain; charset=utf-8", wantText: "ok"},
		{name: "HEAD to a declined GET route is served", method: "HEAD", target: "/deprecated-api/v1/items",
			wantStatus: 200, wantContentType: "text/plain; charset=utf-8", wantText: ""},
		{name: "pattern without methods declines GET", method: "GET", target: "/deprecated-api/v2/items",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("GET", "/deprecated-api/v2/items", "[*]", "/deprecated-api/v2/**")},
		{name: "route with a parameter is declined", method: "GET", target: "/deprecated-api/v1/items/7",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("GET", "/deprecated-api/v1/items/7", "[GET PUT DELETE]", "/deprecated-api/v1/**")},
		{name: "query string is declined with the query in the message", method: "GET", target: "/deprecated-api/v1/items?x=1",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("GET", "/deprecated-api/v1/items?x=1", "[GET PUT DELETE]", "/deprecated-api/v1/**")},
		{name: "trailing slash is declined", method: "GET", target: "/deprecated-api/v1/items/",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("GET", "/deprecated-api/v1/items/", "[GET PUT DELETE]", "/deprecated-api/v1/**")},
		{name: "double leading slash is declined as the normalized path", method: "GET", target: "//deprecated-api/v1/items",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("GET", "/deprecated-api/v1/items", "[GET PUT DELETE]", "/deprecated-api/v1/**")},
		{name: "dot-dot segment is declined as the normalized path", method: "GET", target: "/api/../deprecated-api/v1/items",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("GET", "/deprecated-api/v1/items", "[GET PUT DELETE]", "/deprecated-api/v1/**")},
		{name: "encoded dot-dot segment is declined as the normalized path", method: "GET", target: "/api/%2e%2e/deprecated-api/v1/items",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("GET", "/deprecated-api/v1/items", "[GET PUT DELETE]", "/deprecated-api/v1/**")},
		{name: "percent-encoded letter is declined as the normalized path", method: "GET", target: "/deprecated-api/%761/items",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("GET", "/deprecated-api/v1/items", "[GET PUT DELETE]", "/deprecated-api/v1/**")},
		{name: "exact pattern is declined", method: "GET", target: "/deprecated-api/exact/items",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("GET", "/deprecated-api/exact/items", "[GET]", "/deprecated-api/exact/items")},
		{name: "query string on an exact pattern is declined by the path", method: "GET", target: "/deprecated-api/exact/items?x=1",
			wantStatus: 404, wantContentType: "application/json",
			wantTMF: declined("GET", "/deprecated-api/exact/items?x=1", "[GET]", "/deprecated-api/exact/items")},
		{name: "trailing slash on an exact pattern is served", method: "GET", target: "/deprecated-api/exact/items/",
			wantStatus: 200, wantContentType: "text/plain; charset=utf-8", wantText: "ok"},
		{name: "path outside the patterns is served", method: "GET", target: "/api/v1/items",
			wantStatus: 200, wantContentType: "text/plain; charset=utf-8", wantText: "ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := send(t, addr, rawRequest(tt.method, tt.target, addr))

			assert.Equal(t, tt.wantStatus, result.status)
			assert.Equal(t, tt.wantContentType, result.header.Get("Content-Type"))
			if tt.wantTMF != nil {
				assert.Equal(t, *tt.wantTMF, decodeTMF(t, result.body))
			} else {
				assert.Equal(t, tt.wantText, result.body)
			}
		})
	}
}

func TestDeprecatedApiLogsDisabledRoutesOnListen(t *testing.T) {
	output := captureStdout(t)

	serveDeprecatedApiApp(t)

	want := "Disabling the following deprecated paths: \n" +
		"/deprecated-api/exact/items [GET]\n" +
		"/deprecated-api/v1/items [GET,PUT,DELETE]\n" +
		"/deprecated-api/v1/items/:id [GET]\n" +
		"/deprecated-api/v2/items [GET,HEAD]"
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Contains(c, output.text(), want)
	}, 5*time.Second, 20*time.Millisecond)
}

func TestTracingSpanForRoutedRequest(t *testing.T) {
	prepareBuilderEnvironment(t)
	tracing := newInMemoryTracing(t)
	app, err := New(fiber.Config{ErrorHandler: serverErrors.DefaultErrorHandler(unknownErrorCode)}).
		WithTracer(tracing).
		Process()
	require.NoError(t, err)
	app.Get("/items/:id", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	app.Get("/unavailable", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	})
	app.Get("/broken", func(c fiber.Ctx) error {
		return errors.New("broken handler")
	})
	addr := serve(t, app)
	port := int64(serverPort(t, addr))

	tests := []struct {
		name       string
		target     string
		host       string
		wantAttrs  func(clientPort int64) map[string]any
		wantStatus codes.Code
	}{
		{
			name: "request by address", target: "/items/42?x=1", host: addr, wantStatus: codes.Unset,
			wantAttrs: func(clientPort int64) map[string]any {
				return map[string]any{
					"http.method": "GET", "http.route": "/", "http.target": "/items/42?x=1", "http.url": "/items/42?x=1",
					"http.scheme": "http", "http.server_name": "characterization-server", "http.user_agent": "characterization-agent",
					"http.status_code": int64(200), "net.host.ip": "127.0.0.1", "net.host.name": nil, "net.host.port": port,
					"net.peer.ip": "127.0.0.1", "net.peer.port": clientPort,
				}
			},
		},
		{
			name: "request by host name", target: "/items/42", host: "localhost:" + strconv.FormatInt(port, 10), wantStatus: codes.Unset,
			wantAttrs: func(clientPort int64) map[string]any {
				return map[string]any{
					"http.method": "GET", "http.route": "/", "http.target": "/items/42", "http.url": "/items/42",
					"http.scheme": "http", "http.server_name": "characterization-server", "http.user_agent": "characterization-agent",
					"http.status_code": int64(200), "net.host.ip": nil, "net.host.name": "localhost", "net.host.port": port,
					"net.peer.ip": "127.0.0.1", "net.peer.port": clientPort,
				}
			},
		},
		{
			name: "handler error is recorded before the error handler sets the status", target: "/broken", host: addr, wantStatus: codes.Unset,
			wantAttrs: func(clientPort int64) map[string]any {
				return map[string]any{
					"http.method": "GET", "http.route": "/", "http.target": "/broken", "http.url": "/broken",
					"http.scheme": "http", "http.server_name": "characterization-server", "http.user_agent": "characterization-agent",
					"http.status_code": int64(200), "net.host.ip": "127.0.0.1", "net.host.name": nil, "net.host.port": port,
					"net.peer.ip": "127.0.0.1", "net.peer.port": clientPort,
				}
			},
		},
		{
			name: "status set by the handler is recorded as the span status", target: "/unavailable", host: addr, wantStatus: codes.Error,
			wantAttrs: func(clientPort int64) map[string]any {
				return map[string]any{
					"http.method": "GET", "http.route": "/", "http.target": "/unavailable", "http.url": "/unavailable",
					"http.scheme": "http", "http.server_name": "characterization-server", "http.user_agent": "characterization-agent",
					"http.status_code": int64(503), "net.host.ip": "127.0.0.1", "net.host.name": nil, "net.host.port": port,
					"net.peer.ip": "127.0.0.1", "net.peer.port": clientPort,
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracing.spans.Reset()

			result := send(t, addr, rawRequest("GET", tt.target, tt.host, "User-Agent: characterization-agent"))

			spans := tracing.spans.GetSpans()
			require.Len(t, spans, 1, "spans exported for GET %s", tt.target)
			span := spans[0]
			assert.Equal(t, "/", span.Name)
			assert.Equal(t, tt.wantStatus, span.Status.Code)
			want := tt.wantAttrs(int64(result.clientPort))
			assert.Equal(t, want, spanAttributes(span, want))
		})
	}
}

func TestTracingSkipsUntracedPaths(t *testing.T) {
	prepareBuilderEnvironment(t)
	tracing := newInMemoryTracing(t)
	healthService, err := health.NewHealthService()
	require.NoError(t, err)
	app, err := New().
		WithTracer(tracing, SkipTracing("/skip-me")).
		WithHealth("/health", healthService).
		Process()
	require.NoError(t, err)
	for _, path := range []string{"/static/app.js", "/skip-me", "/traced"} {
		app.Get(path, func(c fiber.Ctx) error {
			return c.SendString("ok")
		})
	}
	addr := serve(t, app)

	tests := []struct {
		target    string
		wantSpans int
	}{
		{target: "/static/app.js", wantSpans: 0},
		{target: "/health", wantSpans: 0},
		{target: "/skip-me", wantSpans: 0},
		{target: "/traced", wantSpans: 1},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			tracing.spans.Reset()

			result := send(t, addr, rawRequest("GET", tt.target, addr))

			require.Equal(t, 200, result.status)
			assert.Len(t, tracing.spans.GetSpans(), tt.wantSpans, "spans exported for GET %s", tt.target)
		})
	}
}

func TestTracingContinuesB3Parent(t *testing.T) {
	prepareBuilderEnvironment(t)
	tracing := newInMemoryTracing(t)
	app, err := New().WithTracer(tracing).Process()
	require.NoError(t, err)
	app.Get("/traced", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	addr := serve(t, app)

	send(t, addr, rawRequest("GET", "/traced", addr,
		"X-B3-TraceId: 463ac35c9f6413ad48485a3953bb6124", "X-B3-SpanId: a2fb4a1d1a96d312", "X-B3-Sampled: 1"))

	spans := tracing.spans.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "463ac35c9f6413ad48485a3953bb6124", spans[0].SpanContext.TraceID().String())
	assert.Equal(t, "a2fb4a1d1a96d312", spans[0].Parent.SpanID().String())
	assert.True(t, spans[0].Parent.IsRemote(), "parent span context %v", spans[0].Parent)
}

func TestPrometheusMiddlewareRecordsMatchedRoutesOnly(t *testing.T) {
	prepareBuilderEnvironment(t)
	registry := usePrometheusRegistry(t)
	app, err := New(fiber.Config{ErrorHandler: serverErrors.DefaultErrorHandler(unknownErrorCode)}).
		WithPrometheus("/prometheus").
		Process()
	require.NoError(t, err)
	app.Get("/items", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	app.Get("/items/:id", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	app.Get("/broken", func(c fiber.Ctx) error {
		return errors.New("broken handler")
	})
	addr := serve(t, app)

	for _, target := range []string{"/broken", "/missing", "/items", "/items/42"} {
		send(t, addr, rawRequest("GET", target, addr))
	}

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		series, err := requestStatusSeries(registry)
		assert.NoError(c, err)
		assert.Equal(c, map[string]float64{
			"method=get path=/items status=200":     1,
			"method=get path=/items/:id status=200": 1,
		}, series)
	}, 5*time.Second, 20*time.Millisecond)
}

func TestPprofListensOnLoopbackOnly(t *testing.T) {
	prepareBuilderEnvironment(t)
	port := freeTCPPort(t)
	_, err := New().WithPprof(strconv.Itoa(port)).Process()
	require.NoError(t, err)
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	indexURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", port)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		resp, err := client.Get(indexURL)
		if assert.NoError(c, err, "GET %s", indexURL) {
			resp.Body.Close()
			assert.Equal(c, 200, resp.StatusCode, "GET %s", indexURL)
		}
	}, 10*time.Second, 50*time.Millisecond)

	tests := []struct {
		name string
		host string
	}{
		{name: "IPv6 loopback", host: "::1"},
		{name: "non-loopback interface", host: nonLoopbackIPv4()},
	}
	for _, tt := range tests {
		t.Run(tt.name+" is refused", func(t *testing.T) {
			if tt.host == "" {
				t.Skip("no non-loopback IPv4 address on this machine")
			}
			address := net.JoinHostPort(tt.host, strconv.Itoa(port))

			conn, err := net.DialTimeout("tcp", address, 2*time.Second)

			if conn != nil {
				conn.Close()
			}
			assert.Error(t, err, "dial %s", address)
		})
	}
}

func TestReadBufferSizeLimitsRequestHeaders(t *testing.T) {
	prepareBuilderEnvironment(t)
	app, err := New(fiber.Config{ErrorHandler: serverErrors.DefaultErrorHandler(unknownErrorCode)}).Process()
	require.NoError(t, err)
	app.Get("/ok", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	addr := serve(t, app)

	tests := []struct {
		name            string
		request         string
		wantStatus      int
		wantContentType string
	}{
		{name: "request head of exactly 10240 bytes is accepted",
			request: paddedRequest(addr, 10240, true), wantStatus: 200, wantContentType: "text/plain; charset=utf-8"},
		{name: "request head not ended within 10240 bytes is refused",
			request: paddedRequest(addr, 10240, false), wantStatus: 431, wantContentType: "application/json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := send(t, addr, tt.request)

			assert.Equal(t, tt.wantStatus, result.status)
			assert.Equal(t, tt.wantContentType, result.header.Get("Content-Type"))
		})
	}
}

func TestProtocolErrorsBeforeRouting(t *testing.T) {
	prepareBuilderEnvironment(t)
	registry := usePrometheusRegistry(t)
	tracing := newInMemoryTracing(t)
	app, err := New(fiber.Config{ErrorHandler: serverErrors.DefaultErrorHandler(unknownErrorCode), BodyLimit: 1024}).
		WithTracer(tracing).
		WithPrometheus("/prometheus").
		Process()
	require.NoError(t, err)
	app.All("/ok", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	addr := serve(t, app)

	tests := []struct {
		name               string
		request            string
		wantStatus         int
		wantStatusField    string
		wantMessagePattern string
		refusedMetric      string
		wantLogPattern     string
	}{
		{
			name:       "declared body over BodyLimit is 413",
			request:    rawRequest("POST", "/ok", addr, "X-Request-Id: protocol-error-request", "Content-Length: 2048"),
			wantStatus: 413, wantStatusField: "413", wantMessagePattern: `^Request Entity Too Large$`,
			refusedMetric:  "method=post path=/ status=413",
			wantLogPattern: `\[request_id=protocol-error-request\].*\[class=errors\].*Request Entity Too Large`,
		},
		{
			name:       "request head over ReadBufferSize is 431",
			request:    paddedRequest(addr, 10240, false),
			wantStatus: 431, wantStatusField: "431", wantMessagePattern: `^Request Header Fields Too Large$`,
			refusedMetric:  "method=get path=/ status=431",
			wantLogPattern: `\[request_id=[0-9.]+\].*\[class=errors\].*Request Header Fields Too Large`,
		},
		{
			name:       "malformed Content-Length is 400",
			request:    rawRequest("GET", "/ok", addr, "X-Request-Id: protocol-error-request", "Content-Length: abc"),
			wantStatus: 400, wantStatusField: "400", wantMessagePattern: `cannot parse content-length`,
			refusedMetric:  "method=get path=/ status=400",
			wantLogPattern: `\[request_id=[0-9.]+\].*\[class=errors\].*cannot parse content-length`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracing.spans.Reset()
			output := captureStdout(t)

			result := send(t, addr, tt.request)

			output.stop()
			assert.Equal(t, tt.wantStatus, result.status)
			assert.Equal(t, "application/json", result.header.Get("Content-Type"))
			response := decodeTMF(t, result.body)
			assert.Equal(t, "TEST-0001", response.Code)
			assert.Equal(t, tt.wantStatusField, response.Status)
			assert.Regexp(t, tt.wantMessagePattern, response.Message)
			assert.Empty(t, tracing.spans.GetSpans(), "spans exported for the refused request")
			series, err := requestStatusSeries(registry)
			require.NoError(t, err)
			assert.NotContains(t, series, tt.refusedMetric, "request metrics")
			assert.Regexp(t, tt.wantLogPattern, output.text(), "error log line written by DefaultErrorHandler")
		})
	}
}

func serveDeprecatedApiApp(t *testing.T) string {
	t.Helper()
	prepareBuilderEnvironment(t)
	configloader.InitWithSourcesArray(configloader.BasePropertySources(
		configloader.YamlPropertySourceParams{ConfigFilePath: "testdata/deprecated-api-disabled.yaml"}))
	t.Cleanup(func() {
		configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
	})
	app, err := New(fiber.Config{ErrorHandler: serverErrors.DefaultErrorHandler(unknownErrorCode)}).
		WithDeprecatedApiSwitchedOff().
		Process()
	require.NoError(t, err)
	ok := func(c fiber.Ctx) error {
		return c.SendString("ok")
	}
	app.Get("/deprecated-api/v1/items", ok)
	app.Put("/deprecated-api/v1/items", ok)
	app.Delete("/deprecated-api/v1/items", ok)
	app.Post("/deprecated-api/v1/items", ok)
	app.Get("/deprecated-api/v1/items/:id", ok)
	app.Get("/deprecated-api/v2/items", ok)
	app.Get("/deprecated-api/exact/items", ok)
	app.Get("/api/v1/items", ok)
	return serve(t, app)
}

func prepareBuilderEnvironment(t *testing.T) {
	t.Helper()
	serviceloader.Register(1, &security.DummyFiberServerSecurityMiddleware{})
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
}

func serve(t *testing.T, app *fiber.App) string {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })
	return ln.Addr().String()
}

func rawRequest(method, target, host string, headers ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n", method, target, host)
	for _, header := range headers {
		b.WriteString(header + "\r\n")
	}
	b.WriteString("\r\n")
	return b.String()
}

func paddedRequest(host string, totalBytes int, terminated bool) string {
	head := "GET /ok HTTP/1.1\r\nHost: " + host + "\r\nConnection: close\r\nX-Request-Id: protocol-error-request\r\nX-Padding: "
	end := ""
	if terminated {
		end = "\r\n\r\n"
	}
	return head + strings.Repeat("a", totalBytes-len(head)-len(end)) + end
}

func send(t *testing.T, addr, request string) httpResult {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))
	_, err = io.WriteString(conn, request)
	require.NoError(t, err)
	requestLine, _, _ := strings.Cut(request, "\r\n")
	method, _, _ := strings.Cut(requestLine, " ")
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: method})
	require.NoError(t, err, "reading the response to %q", requestLine)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading the response body to %q", requestLine)
	return httpResult{
		status:     resp.StatusCode,
		header:     resp.Header,
		body:       string(body),
		clientPort: conn.LocalAddr().(*net.TCPAddr).Port,
	}
}

func decodeTMF(t *testing.T, body string) tmfFields {
	t.Helper()
	var response tmf.Response
	require.NoError(t, json.Unmarshal([]byte(body), &response), "TMF response body %q", body)
	status := ""
	if response.Status != nil {
		status = *response.Status
	}
	return tmfFields{Code: response.Code, Message: response.Message, Status: status}
}

func serverPort(t *testing.T, addr string) int {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	value, err := strconv.Atoi(port)
	require.NoError(t, err)
	return value
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func nonLoopbackIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}
	return ""
}

type inMemoryTracing struct {
	spans *tracetest.InMemoryExporter
}

func newInMemoryTracing(t *testing.T) inMemoryTracing {
	t.Helper()
	previous := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	return inMemoryTracing{spans: tracetest.NewInMemoryExporter()}
}

func (e inMemoryTracing) RegisterTracerProvider() (bool, error) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(e.spans)))
	return true, nil
}

func (e inMemoryTracing) ServerName() string {
	return "characterization-server"
}

func spanAttributes(span tracetest.SpanStub, keys map[string]any) map[string]any {
	values := make(map[string]any, len(keys))
	for key := range keys {
		values[key] = nil
	}
	for _, attr := range span.Attributes {
		if _, wanted := keys[string(attr.Key)]; wanted {
			values[string(attr.Key)] = attr.Value.AsInterface()
		}
	}
	return values
}

func usePrometheusRegistry(t *testing.T) *prometheus.Registry {
	t.Helper()
	registry := prometheus.NewRegistry()
	previousRegisterer, previousGatherer := prometheus.DefaultRegisterer, prometheus.DefaultGatherer
	prometheus.DefaultRegisterer, prometheus.DefaultGatherer = registry, registry
	t.Cleanup(func() {
		prometheus.DefaultRegisterer, prometheus.DefaultGatherer = previousRegisterer, previousGatherer
	})
	return registry
}

func requestStatusSeries(registry *prometheus.Registry) (map[string]float64, error) {
	families, err := registry.Gather()
	if err != nil {
		return nil, err
	}
	series := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "http_request_status" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, pair := range metric.GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
			key := "method=" + labels["method"] + " path=" + labels["path"] + " status=" + labels["status"]
			series[key] = metric.GetCounter().GetValue()
		}
	}
	return series, nil
}

type capturedOutput struct {
	mu       sync.Mutex
	buffer   strings.Builder
	stopOnce sync.Once
	stopFunc func()
}

func captureStdout(t *testing.T) *capturedOutput {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer
	output := &capturedOutput{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		chunk := make([]byte, 4096)
		for {
			n, err := reader.Read(chunk)
			output.mu.Lock()
			output.buffer.Write(chunk[:n])
			output.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	output.stopFunc = func() {
		os.Stdout = original
		_ = writer.Close()
		<-done
		_ = reader.Close()
	}
	t.Cleanup(output.stop)
	return output
}

func (o *capturedOutput) stop() {
	o.stopOnce.Do(o.stopFunc)
}

func (o *capturedOutput) text() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}
