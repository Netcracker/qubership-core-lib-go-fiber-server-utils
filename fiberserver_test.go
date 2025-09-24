package fiberserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/apiversion"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/health"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/monitoring"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/tracing"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/security"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/test"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/acceptlanguage"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/xrequestid"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxhelper"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	zkmodel "github.com/openzipkin/zipkin-go/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TestSuite struct {
	suite.Suite
}

const (
	defaultReadBufferSizeInt = 10240
)

func (suite *TestSuite) SetupSuite() {
	serviceloader.Register(1, &security.DummyFiberServerSecurityMiddleware{})
	test.StartMockServer()
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
}

func (suite *TestSuite) TearDownSuite() {
	test.StopMockServer()
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}

func (suite *TestSuite) TestExampleFiberserver() {
	server := httptest.NewServer(tracingHandler(suite.T()))
	defer server.Close()

	healthService, err := health.NewHealthService()
	assert.Nil(suite.T(), err)
	options := tracing.ZipkinOptions{
		TracingEnabled:             true,
		TracingHost:                server.URL,
		TracingSamplerRateLimiting: 10,
		ServiceName:                "service-name",
		Namespace:                  "namespace",
	}
	pprofPort, err := getFreePort()
	require.Nil(suite.T(), err)
	app, err := New(fiber.Config{DisableKeepalive: true}).
		WithPprof(pprofPort).
		WithHealth("/health", healthService).
		WithPrometheus("/prometheus", monitoring.Config{HttpRequestTimeBuckets: []float64{0.005, 0.01, 0.05, 0.1}}).
		WithTracer(tracing.NewZipkinTracerWithOpts(options)).
		Process()

	assert.Equal(suite.T(), err, nil)

	appPort, err := getFreePort()
	require.Nil(suite.T(), err)

	app.Get("/test", func(ctx *fiber.Ctx) error {
		return ctx.Status(fiber.StatusOK).SendString("I'm test handler!!!")
	})
	listenAndWaitForPort(suite.T(), app, appPort)
	defer func() {
		app.Shutdown()
		time.Sleep(time.Millisecond * 100)
	}()

	resp, err := http.Get("http://localhost:" + appPort + "/test")

	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	assert.Equal(suite.T(), "I'm test handler!!!", string(bodyBytes))
	assert.NotEqual(suite.T(), "", resp.Header.Get(xrequestid.X_REQUEST_ID_HEADER_NAME))

	healthResp, _ := app.Test(httptest.NewRequest("GET", "/prometheus", nil))
	assert.Equal(suite.T(), fiber.StatusOK, healthResp.StatusCode)
}

func tracingHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var spans []zkmodel.SpanModel
		if err := json.NewDecoder(r.Body).Decode(&spans); err != nil {
			t.Error(err)
		}
		assert.Equal(t, 1, len(spans))
		span := spans[0]
		assert.Equal(t, "service-name-namespace", span.LocalEndpoint.ServiceName)
		assert.Equal(t, "/test", span.Tags["http.target"])
		w.WriteHeader(fiber.StatusAccepted)
	}
}

func (suite *TestSuite) TestFiberBuilderPprof() {
	port, err := getFreePort()
	require.Nil(suite.T(), err)

	_, err = New(fiber.Config{DisableKeepalive: true}).
		WithPprof(port).
		Process()

	assert.Equal(suite.T(), err, nil)

	pprofResp, _ := http.Get("http://localhost:" + port + "/debug/pprof/")
	assert.Equal(suite.T(), fiber.StatusOK, pprofResp.StatusCode)
}

func (suite *TestSuite) TestFiberBuilderHealth() {
	healthService, err := health.NewHealthService()
	assert.Nil(suite.T(), err)
	app, err := New(fiber.Config{DisableKeepalive: true}).
		WithHealth("/health", healthService).
		Process()

	assert.Nil(suite.T(), err)
	healthResp, _ := app.Test(httptest.NewRequest("GET", "/health", nil)) //.Get("http://localhost:10002/health")
	assert.Equal(suite.T(), fiber.StatusOK, healthResp.StatusCode)
}

func (suite *TestSuite) TestFiberBuilderApiversion() {
	apiVersionService, err := apiversion.NewApiVersionService(apiversion.ApiVersionConfig{PathToApiVersionInfoFile: "./testdata/api-version-info.json"})
	assert.Nil(suite.T(), err)
	app, err := New(fiber.Config{DisableKeepalive: true}).
		WithApiVersion(apiVersionService).
		Process()

	assert.Nil(suite.T(), err)
	resp, _ := app.Test(httptest.NewRequest("GET", "/api-version", nil)) //.Get("http://localhost:10002/api-version")
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)
}

func (suite *TestSuite) TestFiberBuilderWithContext() {
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), "test-key", "test-value"))
	app, err := New(fiber.Config{DisableKeepalive: true}).
		ProcessWithContext(ctx)

	assert.Nil(suite.T(), err)

	app.Get("/test", func(ctx *fiber.Ctx) error {
		value := ctx.UserContext().Value("test-key")
		return ctx.Status(fiber.StatusOK).SendString(fmt.Sprintf("%s", value))
	})

	app.Get("/test-wait", func(ctx *fiber.Ctx) error {
		select {
		case <-time.After(5 * time.Second):
			return ctx.Status(fiber.StatusOK).SendString("done")
		case <-ctx.UserContext().Done():
			return ctx.Status(fiber.StatusOK).SendString("canceled")
		}
	})

	resp, _ := app.Test(httptest.NewRequest("GET", "/test", nil))
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)
	assert.Equal(suite.T(), "test-value", readToString(resp.Body))

	go time.AfterFunc(100*time.Millisecond, cancel)
	resp, _ = app.Test(httptest.NewRequest("GET", "/test-wait", nil))
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)
	assert.Equal(suite.T(), "canceled", readToString(resp.Body))
}

func (suite *TestSuite) TestFiberBuilderTracer() {
	server := httptest.NewUnstartedServer(tracingHandler(suite.T()))
	tracerHost := "127.0.0.1"
	tracerPort := ":9411"
	l, _ := net.Listen("tcp", tracerHost+tracerPort)
	server.Listener = l
	server.Start()
	defer server.Close()

	options := tracing.ZipkinOptions{
		TracingEnabled:             true,
		TracingHost:                tracerHost,
		TracingSamplerRateLimiting: 10,
		ServiceName:                "service-name",
		Namespace:                  "namespace",
	}
	app, err := New(fiber.Config{DisableKeepalive: true}).
		WithTracer(tracing.NewZipkinTracerWithOpts(options)).
		Process()

	assert.Nil(suite.T(), err)
	app.Get("/test", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})
	_, err = app.Test(httptest.NewRequest("GET", "/test", nil))
	assert.Nil(suite.T(), err)
}

func (suite *TestSuite) TestFiberBuilderContext() {
	port, err := getFreePort()
	require.Nil(suite.T(), err)

	app, err := New(fiber.Config{DisableKeepalive: true}).Process()
	assert.Equal(suite.T(), err, nil)
	requestIdFromOutgoingRequest := ""
	app.Get("/test", func(ctx *fiber.Ctx) error {
		testRequest2, _ := http.NewRequest("GET", "http://localhost:"+port+"/test-context", nil)
		ctxhelper.AddSerializableContextData(ctx.UserContext(), testRequest2.Header.Set)
		resp, err := http.DefaultClient.Do(testRequest2)
		assert.Nil(suite.T(), err)
		assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)
		return ctx.SendStatus(fiber.StatusOK)
	})

	app.Get("/test-context", func(ctx *fiber.Ctx) error {
		requestId := string(ctx.Request().Header.Peek(xrequestid.X_REQUEST_ID_HEADER_NAME))
		assert.NotEmpty(suite.T(), requestId)
		requestIdFromOutgoingRequest = requestId
		assert.Equal(suite.T(), "testLanguage", string(ctx.Request().Header.Peek(acceptlanguage.ACCEPT_LANGUAGE_HEADER_NAME)))
		logger.InfoC(ctx.UserContext(), "test resp")
		return ctx.SendStatus(fiber.StatusOK)
	})

	listenAndWaitForPort(suite.T(), app, port)
	defer app.Shutdown()

	testRequest, _ := http.NewRequest("GET", "http://localhost:"+port+"/test", nil)
	testRequest.Header.Set(acceptlanguage.ACCEPT_LANGUAGE_HEADER_NAME, "testLanguage")

	resp, err := http.DefaultClient.Do(testRequest)
	assert.NoError(suite.T(), err)
	requestIdFromResponse := resp.Header.Get(xrequestid.X_REQUEST_ID_HEADER_NAME)
	assert.NotEmpty(suite.T(), requestIdFromResponse)
	assert.Equal(suite.T(), requestIdFromResponse, requestIdFromOutgoingRequest)
	assert.Nil(suite.T(), err)
	time.Sleep(time.Millisecond * 100)
}

func (suite *TestSuite) TestFiberServerConfig_configDoesNotProvided_configWithDefaultValues() {
	app, err := New().Process()

	assert.Equal(suite.T(), err, nil)
	// Check default values
	assert.Equal(suite.T(), app.Config().ReadBufferSize, defaultReadBufferSizeInt)
}

func (suite *TestSuite) TestFiberServerConfig_configProvidedWithoutOverriddenDefaultValues_configWithDefaultValues() {
	app, err := New(fiber.Config{}).Process()

	assert.Equal(suite.T(), err, nil)
	// Check default values
	assert.Equal(suite.T(), app.Config().ReadBufferSize, defaultReadBufferSizeInt)
}

func (suite *TestSuite) TestFiberServerConfig_configProvidedWithOverriddenDefaultValues_configWithOverriddenValues() {
	// set properties for overriding default values in config
	expectedReadBufferSize := 8192
	app, err := New(fiber.Config{ReadBufferSize: 8192}).Process()

	assert.Equal(suite.T(), err, nil)
	// Check that default values were overridden
	assert.Equal(suite.T(), app.Config().ReadBufferSize, expectedReadBufferSize)
}

func (suite *TestSuite) TestDisableDeprecatedApi() {
	assertions := require.New(suite.T())
	configloader.InitWithSourcesArray(configloader.BasePropertySources(
		configloader.YamlPropertySourceParams{ConfigFilePath: "testdata/deprecated-api-disabled.yaml"}))

	app, err := New(fiber.Config{}).WithDeprecatedApiSwitchedOff().Process()
	assertions.Nil(err)

	app.Get("/deprecated-api/v1/test", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})
	app.Post("/deprecated-api/v1/test", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})
	app.Get("/deprecated-api/v2/test", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})
	app.Get("/deprecated-api/v3/test", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})

	port, err := getFreePort()
	require.Nil(suite.T(), err)
	listenAndWaitForPort(suite.T(), app, port)
	defer app.Shutdown()

	request, _ := http.NewRequest("GET", "http://localhost:"+port+"/deprecated-api/v1/test", nil)
	resp1get, err := http.DefaultClient.Do(request)
	assert.NoError(suite.T(), err)
	assertions.NotNil(resp1get)
	assertions.Equal(404, resp1get.StatusCode)

	request, _ = http.NewRequest("POST", "http://localhost:"+port+"/deprecated-api/v1/test", nil)
	resp1post, err := http.DefaultClient.Do(request)
	assert.NoError(suite.T(), err)
	assertions.NotNil(resp1post)
	assertions.Equal(200, resp1post.StatusCode)

	request, _ = http.NewRequest("GET", "http://localhost:"+port+"/deprecated-api/v2/test", nil)
	resp2, err := http.DefaultClient.Do(request)
	assert.NoError(suite.T(), err)
	assertions.NotNil(resp2)
	assertions.Equal(404, resp2.StatusCode)

	request, _ = http.NewRequest("GET", "http://localhost:"+port+"/deprecated-api/v3/test", nil)
	resp3, err := http.DefaultClient.Do(request)
	assert.NoError(suite.T(), err)
	assertions.NotNil(resp3)
	assertions.Equal(200, resp3.StatusCode)
}

func (suite *TestSuite) TestDeprecatedApiDisabledFalse() {
	assertions := require.New(suite.T())
	configloader.InitWithSourcesArray(configloader.BasePropertySources(
		configloader.YamlPropertySourceParams{ConfigFilePath: "testdata/deprecated-api-enabled.yaml"}))

	app, err := New(fiber.Config{}).WithDeprecatedApiSwitchedOff().Process()
	assertions.Nil(err)

	app.Get("/deprecated-api/v1/test", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})
	app.Post("/deprecated-api/v1/test", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})
	app.Get("/deprecated-api/v2/test", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})
	app.Get("/deprecated-api/v3/test", func(ctx *fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	})

	port, err := getFreePort()
	require.Nil(suite.T(), err)
	listenAndWaitForPort(suite.T(), app, port)
	defer app.Shutdown()

	request, _ := http.NewRequest("GET", "http://localhost:"+port+"/deprecated-api/v1/test", nil)
	resp1get, err := http.DefaultClient.Do(request)
	assert.NoError(suite.T(), err)
	assertions.NotNil(resp1get)
	assertions.Equal(200, resp1get.StatusCode)

	request, _ = http.NewRequest("POST", "http://localhost:"+port+"/deprecated-api/v1/test", nil)
	resp1post, err := http.DefaultClient.Do(request)
	assert.NoError(suite.T(), err)
	assertions.NotNil(resp1post)
	assertions.Equal(200, resp1post.StatusCode)

	request, _ = http.NewRequest("GET", "http://localhost:"+port+"/deprecated-api/v2/test", nil)
	resp2, err := http.DefaultClient.Do(request)
	assert.NoError(suite.T(), err)
	assertions.NotNil(resp2)
	assertions.Equal(200, resp2.StatusCode)

	request, _ = http.NewRequest("GET", "http://localhost:"+port+"/deprecated-api/v3/test", nil)
	resp3, err := http.DefaultClient.Do(request)
	assert.NoError(suite.T(), err)
	assertions.NotNil(resp3)
	assertions.Equal(200, resp3.StatusCode)
}

func (suite *TestSuite) TestFiberBuilderLogLevelsInfo() {
	var loggerName = "loggername"
	logger := logging.GetLogger(loggerName)
	logger.SetLevel(logging.LvlCrit)
	app, err := New(fiber.Config{DisableKeepalive: true}).
		WithLogLevelsInfo().
		Process()

	assert.Nil(suite.T(), err)
	resp, _ := app.Test(httptest.NewRequest("GET", "/api/logging/v1/levels", nil))
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)

	var logLevels logging.LogLevels
	if err := json.NewDecoder(resp.Body).Decode(&logLevels); err != nil {
		suite.T().Error(err)
	}
	assert.Equal(suite.T(), strings.ToUpper(logging.LvlCrit.String()), logLevels[loggerName])
}

func readToString(stream io.Reader) string {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(stream)
	if err != nil {
		return ""
	}
	return buf.String()
}

func getFreePort() (string, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return "", err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return "", err
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port), nil
}

func listenAndWaitForPort(t *testing.T, app *fiber.App, port string) {
	go app.Listen(":" + port)
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start listening on port %s in time: %v", port, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
