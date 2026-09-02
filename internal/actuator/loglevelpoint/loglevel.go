package loglevelpoint

import (
	"net/http"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/loglevel"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
)

var logger logging.Logger

func init() {
	logger = logging.GetLogger("fiberloglevelsinfo")
}

func EnableLogLevel(logLevelService loglevel.LogLevelService) fiber.Handler {
	logger.Debug("Starting log level service")
	return addEndpoint(logLevelService)
}

func addEndpoint(logLevelService loglevel.LogLevelService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		data, err := logLevelService.GetLogLevels()
		if err == nil {
			return c.Status(http.StatusOK).JSON(data)
		} else {
			return c.Status(http.StatusInternalServerError).SendString(err.Error())
		}
	}
}
