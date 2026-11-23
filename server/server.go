package server

import (
	"crypto/tls"
	"strings"

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

// StartServerOnAddress serves app on listenAddress and blocks until the server stops. When utils.IsTlsEnabled
// reports true, the server uses the TLS configuration from utils.GetTlsConfig.
//
// config passes listener options such as ListenerNetwork to Fiber. StartServerOnAddress panics if config sets a TLS
// option other than a TLSMinVersion of TLS 1.2, or if the server cannot start.
func StartServerOnAddress(app *fiber.App, listenAddress string, config ...fiber.ListenConfig) {
	if len(config) > 0 {
		if fields := tlsFields(config[0]); len(fields) > 0 {
			logger.Panic("Cannot start server on address=%s: fiber.ListenConfig sets the TLS options %s, but TLS is configured by utils.GetTlsConfig(); leave them unset",
				listenAddress, strings.Join(fields, ", "))
		}
	}
	if utils.IsTlsEnabled() {
		// tcp4 is also the default of fiber.ListenConfig.ListenerNetwork, so the TLS and plain listeners bind the same network.
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

func tlsFields(config fiber.ListenConfig) []string {
	var fields []string
	if config.CertFile != "" {
		fields = append(fields, "CertFile")
	}
	if config.CertKeyFile != "" {
		fields = append(fields, "CertKeyFile")
	}
	if config.CertClientFile != "" {
		fields = append(fields, "CertClientFile")
	}
	if config.TLSConfig != nil {
		fields = append(fields, "TLSConfig")
	}
	if config.TLSConfigFunc != nil {
		fields = append(fields, "TLSConfigFunc")
	}
	if config.AutoCertManager != nil {
		fields = append(fields, "AutoCertManager")
	}
	// TLS 1.2 is the minimum version that utils.GetTlsConfig sets.
	if config.TLSMinVersion != 0 && config.TLSMinVersion != tls.VersionTLS12 {
		fields = append(fields, "TLSMinVersion")
	}
	return fields
}
