package server

import (
	"crypto/tls"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
)

var logger = logging.GetLogger("server")

func StartServer(app *fiber.App, listenAddressKey string) {
	defaultListenAddress := ":8080"
	if utils.IsTlsEnabled() {
		defaultListenAddress = ":8443"
	}
	listenAddress := configloader.GetOrDefaultString(listenAddressKey, defaultListenAddress)
	StartServerOnAddress(app, listenAddress)
}

func StartServerOnAddress(app *fiber.App, listenAddress string) {
	if utils.IsTlsEnabled() {
		ln, err := tls.Listen(app.Config().Network, listenAddress, utils.GetTlsConfig())
		if err != nil {
			logger.Panic("Cannot create listener on address=%s, error=%+v", listenAddress, err)
		}
		if err := app.Listener(ln); err != nil {
			logger.Panic("Cannot start tls listener on address=%s, error=%+v", listenAddress, err)
		}
	} else {
		if err := app.Listen(listenAddress); err != nil {
			logger.Panic("Cannot start listener on address=%s, error=%+v", listenAddress, err)
		}
	}
}
