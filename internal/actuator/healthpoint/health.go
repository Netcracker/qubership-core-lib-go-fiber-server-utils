package healthpoint

import (
	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/health"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
)

var logger logging.Logger

func init() {
	logger = logging.GetLogger("fiberhlth")
}

func EnableHealth(healthService health.HealthService) fiber.Handler {
	logger.Debug("Starting health services")
	healthService.Start()
	return healthMiddleware(healthService)

}
func healthMiddleware(healthService health.HealthService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		data := healthService.GetHealth()
		return c.Status(data.GetStatusCode()).
			JSON(data.GetHealthMap())
	}
}
