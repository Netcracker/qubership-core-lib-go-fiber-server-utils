package server

import (
	"crypto/tls"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
)

var logger = logging.GetLogger("server")

func StartServer(app *fiber.App, listenAddressKey string, config ...fiber.ListenConfig) {
	defaultListenAddress := ":8080"
	if utils.IsTlsEnabled() {
		defaultListenAddress = ":8443"
	}
	listenAddress := configloader.GetOrDefaultString(listenAddressKey, defaultListenAddress)
	StartServerOnAddress(app, listenAddress, config...)
}

func StartServerOnAddress(app *fiber.App, listenAddress string, config ...fiber.ListenConfig) {
	if utils.IsTlsEnabled() {
		network := fiber.NetworkTCP4
		if len(config) > 0 && config[0].ListenerNetwork != "" {
			network = config[0].ListenerNetwork
		}
		ln, err := tls.Listen(network, listenAddress, utils.GetTlsConfig())
		if err != nil {
			logger.Panic("Cannot create listener on address=%s, error=%+v", listenAddress, err)
		}
		if err := app.Listener(ln, config...); err != nil {
			logger.Panic("Cannot start tls listener on address=%s, error=%+v", listenAddress, err)
		}
	} else {
		if err := app.Listen(listenAddress, config...); err != nil {
			logger.Panic("Cannot start listener on address=%s, error=%+v", listenAddress, err)
		}
	}
}
