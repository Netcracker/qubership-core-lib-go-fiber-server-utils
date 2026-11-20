package pprofpoint

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/pprof"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/server"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
)

var logger logging.Logger

func init() {
	logger = logging.GetLogger("fiberpprf")
}

func EnablePprofOnPort(port string) {
	go func() {
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(pprof.New())
		addr := "127.0.0.1:" + port
		logger.Debugf("run pprof on %s", addr)
		server.StartServerOnAddress(app, addr)
	}()
}
