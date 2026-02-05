package apiversionpoint

import (
	"net/http"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/apiversion"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
)

var logger logging.Logger

func init() {
	logger = logging.GetLogger("fiberapiversion")
}

func EnableApiVersion(apiVersionService apiversion.ApiVersionService) fiber.Handler {
	logger.Debug("Starting apiversion services")
	return addEndpoint(apiVersionService)

}
func addEndpoint(apiVersionService apiversion.ApiVersionService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		data, err := apiVersionService.GetApiVersion(ctx)
		if err == nil {
			return c.Status(http.StatusOK).JSON(data)
		} else {
			return c.Status(http.StatusInternalServerError).SendString(err.Error())
		}
	}
}
