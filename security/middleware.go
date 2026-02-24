package security

import (
	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
)

var logger logging.Logger

type SecurityMiddleware interface {
	GetSecurityMiddleware() func(c *fiber.Ctx) error
}

type DummyFiberServerSecurityMiddleware struct {
}

func init() {
	logger = logging.GetLogger("fiberserver")
}

func (m *DummyFiberServerSecurityMiddleware) GetSecurityMiddleware() func(c *fiber.Ctx) error {
	logger.Info("Security middleware is not active by default")
	return func(c *fiber.Ctx) error {
		return c.Next()
	}
}
