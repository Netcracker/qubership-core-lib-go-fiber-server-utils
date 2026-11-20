package server

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/test"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const port = ":10001"

type TestSuite struct {
	suite.Suite
}

func (suite *TestSuite) SetupSuite() {
	test.StartMockServer()
	os.Setenv("http.server.bind", port)
	os.Setenv("log.level", "debug")
	os.Setenv("microservice.namespace", "namespace")
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
}

func (suite *TestSuite) TearDownSuite() {
	test.StopMockServer()
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(TestSuite))
}

func (suite *TestSuite) TestStartServer() {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/test", func(ctx *fiber.Ctx) error {
		return ctx.Status(fiber.StatusOK).SendString("I'm test handler!!!")
	})
	go func() {
		StartServer(app, "http.server.bind")
	}()
	var response *http.Response
	for i := 0; i < 4; i++ {
		var err error
		response, err = http.Get("http://127.0.0.1" + port + "/test")
		fmt.Println(response)
		fmt.Println(err)
		if err != nil || response.StatusCode != 200 {
			time.Sleep(500 * time.Millisecond)
		} else {
			break
		}
	}
	defer response.Body.Close()
	assert.Equal(suite.T(), 200, response.StatusCode)
}
