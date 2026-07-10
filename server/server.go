package server

import (
	"crypto/tls"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
)

var logger = logging.GetLogger("server")

func StartServer(app *fiber.App, listenAddressKey string, listenConfig ...fiber.ListenConfig) {
	defaultListenAddress := ":8080"
	if utils.IsTlsEnabled() {
		defaultListenAddress = ":8443"
	}
	listenAddress := configloader.GetOrDefaultString(listenAddressKey, defaultListenAddress)
	StartServerOnAddress(app, listenAddress, listenConfig...)
}

func StartServerOnAddress(app *fiber.App, listenAddress string, listenConfig ...fiber.ListenConfig) {
	if utils.IsTlsEnabled() {
		ln, err := tls.Listen(listenerNetwork(listenConfig), listenAddress, utils.GetTlsConfig())
		if err != nil {
			logger.Panic("Cannot create listener on address=%s, error=%+v", listenAddress, err)
		}
		if err := app.Listener(ln, listenConfig...); err != nil {
			logger.Panic("Cannot start tls listener on address=%s, error=%+v", listenAddress, err)
		}
	} else {
		if err := app.Listen(listenAddress, listenConfig...); err != nil {
			logger.Panic("Cannot start listener on address=%s, error=%+v", listenAddress, err)
		}
	}
}

// listenerNetwork returns the network for the TLS listener, honoring an explicit
// ListenerNetwork from the caller's ListenConfig and defaulting to fiber's tcp4,
// matching the non-TLS app.Listen default.
func listenerNetwork(listenConfig []fiber.ListenConfig) string {
	if len(listenConfig) > 0 && listenConfig[0].ListenerNetwork != "" {
		return listenConfig[0].ListenerNetwork
	}
	return fiber.NetworkTCP4
}
