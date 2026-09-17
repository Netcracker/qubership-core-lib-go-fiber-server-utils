package actuator

import (
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/apiversion"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/health"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/monitoring"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/internal/actuator/apiversionpoint"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/internal/actuator/healthpoint"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/internal/actuator/monitorpoint"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/internal/actuator/pprofpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const healthResponse = `{"status":"UP"}`
const hostAddr = "localhost:3000"

type ExampleTestSuite struct {
	suite.Suite
	app *fiber.App
}

func (suite *ExampleTestSuite) SetupSuite() {
	suite.app = app()
	go func() {
		if err := suite.app.Listen(hostAddr); err != nil {
			log.Panic(err)
		}
	}()
	ready := false
	var response *http.Response
	for i := 0; i < 4; i++ {
		var err error
		response, err = http.Get("http://" + hostAddr + "/health")
		if err != nil || response.StatusCode != 200 {
			time.Sleep(500 * time.Millisecond)
		} else {
			ready = true
			break
		}
	}
	defer response.Body.Close()
	assert.True(suite.T(), ready)
}

func (suite *ExampleTestSuite) TearDownSuite() {
	go func() {
		suite.app.Shutdown()
	}()
}

func (suite *ExampleTestSuite) TestHealth() {
	response, err := http.Get("http://" + hostAddr + "/health")
	assert.Nil(suite.T(), err)
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	bodyString := string(body)
	assert.Equal(suite.T(), 200, response.StatusCode)
	assert.Equal(suite.T(), healthResponse, bodyString)
}

func (suite *ExampleTestSuite) TestPprof() {
	var response *http.Response
	for i := 0; i < 4; i++ {
		var err error
		response, err = http.Get("http://localhost:6060/debug/pprof/")
		if err != nil || response.StatusCode != 200 {
			time.Sleep(500 * time.Millisecond)
		} else {
			break
		}
	}
	defer response.Body.Close()
	assert.Equal(suite.T(), 200, response.StatusCode)
}

func (suite *ExampleTestSuite) TestPrometheusMetrics() {
	response, err := http.Get("http://" + hostAddr + "/prometheus")
	assert.Nil(suite.T(), err)
	defer response.Body.Close()
	assert.Equal(suite.T(), 200, response.StatusCode)
}

func (suite *ExampleTestSuite) TestApiVersion() {
	response, err := http.Get("http://" + hostAddr + "/api-version")
	assert.Nil(suite.T(), err)
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	bodyString := string(body)
	assert.Equal(suite.T(), 200, response.StatusCode)
	assert.True(suite.T(), strings.Contains(bodyString, "bluegreen"))
}

func TestSuite(t *testing.T) {
	suite.Run(t, new(ExampleTestSuite))
}

func app() *fiber.App {
	app := fiber.New()

	pprofpoint.EnablePprofOnPort("6060")

	healthService, _ := health.NewHealthService()
	app.Get("/health", healthpoint.EnableHealth(healthService))
	apiVersionService, _ := apiversion.NewApiVersionService(apiversion.ApiVersionConfig{PathToApiVersionInfoFile: "../../testdata/api-version-info.json"})
	app.Get("/api-version", apiversionpoint.EnableApiVersion(apiVersionService))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello, World!")
	})

	if err := monitorpoint.EnablePrometheus("/prometheus", &monitoring.Config{}, app); err != nil {
		panic(err)
	}

	return app
}
