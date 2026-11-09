package deprecatedapi

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go-error-handling/v3/tmf"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/vibrantbyte/go-antpath/antpath"
)

type TestSuite struct {
	suite.Suite
}

var propertyFilePathEnvValue = os.Getenv("PROPERTY_FILE_PATH")
var ErrorMsg = "is declined with 404 Not Found, because the following deprecated REST API is disabled"

var app *fiber.App

func (suite *TestSuite) SetupSuite() {
	os.Setenv("PROPERTY_FILE_PATH", "test/")
	os.Setenv("DEPRECATED_API_DISABLED", "true")
	configloader.InitWithSourcesArray(configloader.BasePropertySources())
	app = fiber.New()
	DisableDeprecatedApi(app)
	registerHandlers(app)
}

func (suite *TestSuite) TearDownSuite() {
	os.Setenv("PROPERTY_FILE_PATH", propertyFilePathEnvValue)
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}

func (suite *TestSuite) TestV1() {
	assert := require.New(suite.T())

	test(assert, app, "GET", "/deprecated-api/v1/test", 404, ErrorMsg)
	test(assert, app, "POST", "/deprecated-api/v1/test", 200, "ok")
	test(assert, app, "GET", "/deprecated-api/v1/test/inner", 404, ErrorMsg)
	test(assert, app, "GET", "/deprecated-api/v1/test/inner/wildcard", 404, ErrorMsg)
	test(assert, app, "GET", "/deprecated-api/v1/test/inner/wildcard-plus/plus", 404, ErrorMsg)
	test(assert, app, "GET", "/deprecated-api/v1/test/inner/extension/test.html", 404, ErrorMsg)
}

func (suite *TestSuite) TestV2() {
	assert := require.New(suite.T())

	test(assert, app, "GET", "/deprecated-api/v2/test", 404, ErrorMsg)
	test(assert, app, "POST", "/deprecated-api/v2/test", 404, ErrorMsg)
	test(assert, app, "GET", "/deprecated-api/v2/test/inner", 404, ErrorMsg)
	test(assert, app, "GET", "/deprecated-api/v2/test/inner/wildcard", 404, ErrorMsg)
	test(assert, app, "GET", "/deprecated-api/v2/test/inner/wildcard-plus/plus", 404, ErrorMsg)
	test(assert, app, "GET", "/deprecated-api/v2/test/inner/extension/test.html", 404, ErrorMsg)
}

func (suite *TestSuite) TestV3() {
	assert := require.New(suite.T())

	test(assert, app, "GET", "/deprecated-api/v3/test", 200, "ok")
	test(assert, app, "POST", "/deprecated-api/v3/test", 200, "ok")
	test(assert, app, "GET", "/deprecated-api/v3/test/inner", 200, "ok")
	test(assert, app, "GET", "/deprecated-api/v3/test/inner/wildcard", 200, "ok")
	test(assert, app, "GET", "/deprecated-api/v3/test/inner/wildcard-plus/plus", 200, "ok")
	test(assert, app, "GET", "/deprecated-api/v3/test/inner/extension/test.html", 200, "ok")
}

func test(assert *require.Assertions, app *fiber.App, method string, uri string, expectedCode int, expectedStr string) {
	req := httptest.NewRequest(method, uri, nil)
	resp, _ := app.Test(req, -1)
	assert.Equal(expectedCode, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	assert.Nil(err)
	if expectedCode >= 300 {
		tmfResp := tmf.Response{}
		err = json.Unmarshal(respBody, &tmfResp)
		assert.Nil(err)
		assert.True(strings.Contains(tmfResp.Message, expectedStr))
	} else {
		assert.True(strings.Contains(string(respBody), expectedStr))
	}
}

func OkHandler(ctx *fiber.Ctx) error {
	return ctx.Status(200).Send([]byte("ok"))
}

func registerHandlers(app *fiber.App) {
	v1 := app.Group("/deprecated-api/v1/test")
	v1.Get("", OkHandler)
	v1.Post("", OkHandler)
	v1.Get("/inner", OkHandler)
	v1.Get("/inner/wildcard/:param?", OkHandler)
	v1.Get("/inner/wildcard/:param1?.:param2", OkHandler)
	v1.Get("/inner/wildcard/:param1?-:param2", OkHandler)
	v1.Get("/inner/wildcard-plus/+", OkHandler)
	v1.Get("/inner/wildcard-star/*", OkHandler)
	v1.Get("/inner/extension/:name.html", OkHandler)

	v2 := app.Group("/deprecated-api/v2/test")
	v2.Get("", OkHandler)
	v2.Post("", OkHandler)
	v2.Get("/inner", OkHandler)
	v2.Get("/inner/wildcard/:param?", OkHandler)
	v2.Get("/inner/wildcard/:param1?.:param2", OkHandler)
	v2.Get("/inner/wildcard/:param1?-:param2", OkHandler)
	v2.Get("/inner/wildcard-plus/+", OkHandler)
	v2.Get("/inner/wildcard-star/*", OkHandler)
	v2.Get("/inner/extension/:name.html", OkHandler)

	v3 := app.Group("/deprecated-api/v3/test")
	v3.Get("", OkHandler)
	v3.Post("", OkHandler)
	v3.Get("/inner", OkHandler)
	v3.Get("/inner/wildcard/:param?", OkHandler)
	v3.Get("/inner/wildcard/:param1?.:param2", OkHandler)
	v3.Get("/inner/wildcard/:param1?-:param2", OkHandler)
	v3.Get("/inner/wildcard-plus/+", OkHandler)
	v3.Get("/inner/wildcard-star/*", OkHandler)
	v3.Get("/inner/extension/:name.html", OkHandler)
}

func (suite *TestSuite) TestGetDisabledEndpoints_Basic() {
	app := fiber.New()

	// Define routes
	app.Get("/api/users/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Post("/api/users/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Delete("/api/items", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendString("ok") })

	// Disabled patterns simulate regex-to-path simplification:
	disabled := &DisabledUrlPatterns{
		urlsMap: map[string][]string{
			"/api/users/param": {"GET", "POST"},
			"/api/items":       {"DELETE"},
		},
		antPathPattern: antpath.New(),
	}

	result := getDisabledEndpoints(app, disabled)

	assert.Len(suite.T(), result, 2)
	assert.Equal(suite.T(), []string{
		"/api/items [DELETE]",
		"/api/users/:id [GET,POST]",
	}, result)
}

func (suite *TestSuite) TestGetDisabledEndpoints_NoMatches() {
	app := fiber.New()
	app.Get("/active", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Post("/something", func(c *fiber.Ctx) error { return c.SendString("ok") })

	disabled := &DisabledUrlPatterns{
		urlsMap: map[string][]string{
			"/notfound": {"GET"},
		},
		antPathPattern: antpath.New(),
	}

	result := getDisabledEndpoints(app, disabled)
	assert.Empty(suite.T(), result)
}

func (suite *TestSuite) TestGetDisabledEndpoints_MultipleGroupsAndDupMethods() {
	app := fiber.New()

	app.Get("/v1/data/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/v2/data/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })

	disabled := &DisabledUrlPatterns{
		urlsMap: map[string][]string{
			"/v1/data/param": {"GET"},
			"/v2/data/param": {"GET"},
		},
		antPathPattern: antpath.New(),
	}

	result := getDisabledEndpoints(app, disabled)
	assert.ElementsMatch(suite.T(), []string{
		"/v1/data/:id [GET]",
		"/v2/data/:id [GET]",
	}, result)
}
