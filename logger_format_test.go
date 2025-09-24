package fiberserver

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/security"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/test"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/tenant"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/xrequestid"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type LoggerSuite struct {
	suite.Suite
}

const (
	x_request_id_value = "11"
	tenant_id_value    = "22"
	placeholder        = "-"
)

func init() {
	os.Setenv("log.level", "debug")
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
}

func (suite *LoggerSuite) SetupSuite() {
	serviceloader.Register(1, &security.DummyFiberServerSecurityMiddleware{})
	test.StartMockServer()
}

func (suite *LoggerSuite) TearDownSuite() {
	test.StopMockServer()
	os.Unsetenv("log.level")
}

func TestLoggerSuite(t *testing.T) {
	suite.Run(t, new(LoggerSuite))
}

func (suite *LoggerSuite) TestGetLoggerRequestId() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, xrequestid.X_REQUEST_ID_COTEXT_NAME, xrequestid.NewXRequestIdContextObject(x_request_id_value))
	assert.Equal(suite.T(), x_request_id_value, getRequestId(ctx))
}

func (suite *LoggerSuite) TestGetEmptyLoggerRequestId() {
	ctx := context.Background()
	assert.Equal(suite.T(), placeholder, getRequestId(ctx))
}

func (suite *LoggerSuite) TestGetLoggerTenantId() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, tenant.TenantContextName, tenant.NewTenantContextObject(tenant_id_value))
	assert.Equal(suite.T(), tenant_id_value, getTenantId(ctx))
}

func (suite *LoggerSuite) TestGetEmptyLoggerTenantId() {
	ctx := context.Background()
	assert.Equal(suite.T(), placeholder, getTenantId(ctx))
}

func (suite *LoggerSuite) TestFiberLoggerFormat() {
	app, err := New(fiber.Config{DisableKeepalive: true}).Process()
	assert.Nil(suite.T(), err)

	app.Get("test", func(c *fiber.Ctx) error {
		oldStdOut := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		logger.DebugC(c.UserContext(), "test-message")

		w.Close()
		out, err := io.ReadAll(r)
		assert.NoError(suite.T(), err)
		os.Stdout = oldStdOut // restoring the real stdout
		logger.Debug("AAAAAAAAAA" + string(out))
		assert.Contains(suite.T(), string(out), "[request_id="+x_request_id_value+"] [tenant_id="+tenant_id_value+"] [thread=-] [class=fiberserver] test-message")
		return nil
	})

	go app.Listen(":10001")

	defer func() {
		app.Shutdown()
		time.Sleep(time.Millisecond * 100)
	}()

	testRequest, err := http.NewRequest(http.MethodGet, "http://localhost:10001/test", nil)
	assert.Nil(suite.T(), err)
	testRequest.Header.Set(xrequestid.X_REQUEST_ID_HEADER_NAME, x_request_id_value)
	testRequest.Header.Set(tenant.TenantHeader, tenant_id_value)
	resp, err := http.DefaultClient.Do(testRequest)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)
}

func (suite *LoggerSuite) TestFiberLoggerFormat_CustomLogFields() {
	app, err := New(fiber.Config{DisableKeepalive: true}).Process()
	assert.Nil(suite.T(), err)

	logging.DefaultFormat.SetCustomLogFields("[custom_field=%{custom_field}] [absent_custom_field=%{absent_custom_field}]")

	app.Get("test", func(c *fiber.Ctx) error {
		oldStdOut := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		c.SetUserContext(context.WithValue(c.UserContext(), "custom_field", "custom_value"))

		logger.DebugC(c.UserContext(), "test-message")

		w.Close()
		out, err := io.ReadAll(r)
		assert.NoError(suite.T(), err)
		os.Stdout = oldStdOut // restoring the real stdout

		assert.Contains(suite.T(), string(out), "[request_id="+x_request_id_value+"] [tenant_id="+tenant_id_value+"] [thread=-] [class=fiberserver] [custom_field=custom_value] [absent_custom_field=-] test-message")
		return nil
	})

	go app.Listen(":10001")

	defer func() {
		app.Shutdown()
		time.Sleep(time.Millisecond * 100)
	}()

	testRequest, err := http.NewRequest(http.MethodGet, "http://localhost:10001/test", nil)
	assert.Nil(suite.T(), err)
	testRequest.Header.Set(xrequestid.X_REQUEST_ID_HEADER_NAME, x_request_id_value)
	testRequest.Header.Set(tenant.TenantHeader, tenant_id_value)
	resp, err := http.DefaultClient.Do(testRequest)
	assert.Equal(suite.T(), fiber.StatusOK, resp.StatusCode)
}
