package fiberserver

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/apiversion"
	clpropertyutils "github.com/netcracker/qubership-core-lib-go-actuator-common/v2/configloader-property-utils"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/health"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/loglevel"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/monitoring"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/tracing"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/deprecatedapi"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/internal/actuator/apiversionpoint"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/internal/actuator/healthpoint"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/internal/actuator/loglevelpoint"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/internal/actuator/monitorpoint"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/internal/actuator/pprofpoint"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/internal/actuator/tracingpoint"
	"github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/security"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxhelper"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxmanager"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
)

var logger logging.Logger

func init() {
	logger = logging.GetLogger("fiberserver")
}

type Builder struct {
	configs                []fiber.Config
	pprofPort              string
	health                 *builderHealth
	apiVersionService      apiversion.ApiVersionService
	prometheusURL          string
	prometheusConfig       monitoring.Config
	exporter               tracing.OpenTelemetryExporter
	switchOffDeprecatedApi bool
	logLevelService        loglevel.LogLevelService
}

type builderHealth struct {
	url           string
	healthService health.HealthService
}

func New(config ...fiber.Config) *Builder {
	return &Builder{configs: config}
}

func (builder *Builder) WithHealth(url string, healthService health.HealthService) *Builder {
	logger.Debug("health indicator will be enabled and available by endpoint = %s", url)
	builder.health = &builderHealth{url: url, healthService: healthService}
	return builder
}

func (builder *Builder) WithPrometheus(url string, config ...monitoring.Config) *Builder {
	logger.Debug("prometheus will be enabled and available by endpoint = %s", url)
	builder.prometheusURL = url
	if len(config) > 0 {
		builder.prometheusConfig = config[0]
	}
	return builder
}

func (builder *Builder) WithTracer(exporter tracing.OpenTelemetryExporter) *Builder {
	logger.Debug("tracer will be enabled")
	builder.exporter = exporter
	return builder
}

func (builder *Builder) WithApiVersion(apiVersionServices ...apiversion.ApiVersionService) *Builder {
	logger.Debug("apiversion will be enabled")
	if len(apiVersionServices) == 1 {
		builder.apiVersionService = apiVersionServices[0]
	} else {
		config := apiversion.ApiVersionConfig{}
		apiVersionService, _ := apiversion.NewApiVersionService(config)
		builder.apiVersionService = apiVersionService
	}
	return builder
}

func (builder *Builder) WithPprof(port string) *Builder {
	logger.Debug("pprof will be enabled and available by URL = localhost:%s", port)
	builder.pprofPort = port
	return builder
}

func (builder *Builder) WithDeprecatedApiSwitchedOff() *Builder {
	logger.Debug("deprecated api switch off feature will be enabled")
	builder.switchOffDeprecatedApi = true
	return builder
}

func (builder *Builder) WithLogLevelsInfo() *Builder {
	logger.Debug("Log levels info endpoint will be enabled")
	builder.logLevelService, _ = loglevel.NewLogLevelService()
	return builder
}

func (builder *Builder) ProcessWithContext(ctx context.Context) (*fiber.App, error) {
	builder.initConfigsWithDefaultValues()
	app := fiber.New(builder.configs...)

	// enable core context
	ctxmanager.Register(baseproviders.Get())
	app.Use(contextInitializer(ctx), responseEnricher())
	// enable instrumental endpoints (health, metrics, tracer, pprof)
	err := builder.enableActuatorEndpoints(app)
	if err != nil {
		logger.Error(err.Error())
		return nil, err
	}

	// enable security logic
	enableSecurity(app)
	// switch off deprecated api
	if builder.switchOffDeprecatedApi {
		if dErr := deprecatedapi.DisableDeprecatedApi(app); dErr != nil {
			return nil, dErr
		}
	}
	logger.Info("fiber.App successfully created")
	return app, nil

}

func (builder *Builder) Process() (*fiber.App, error) {
	return builder.ProcessWithContext(context.Background())
}

func (builder *Builder) initConfigsWithDefaultValues() {
	if len(builder.configs) == 0 {
		builder.configs = make([]fiber.Config, 1)
		builder.configs[0] = fiber.Config{}
	}
	// only first config have further processing by Fiber (it is Fiber implementation for optional argument "config")
	setDefaultValuesToConfig(&builder.configs[0])
}

func setDefaultValuesToConfig(config *fiber.Config) {
	if config.ReadBufferSize <= 0 {
		config.ReadBufferSize = clpropertyutils.GetHttpBufferHeaderMaxSizeBytes()
		logger.Info("HTTP buffer header Max size has been set to %d bytes", config.ReadBufferSize)
	}
}

func enableSecurity(app *fiber.App) {
	securityMiddleware := serviceloader.MustLoad[security.SecurityMiddleware]()
	app.Use(securityMiddleware.GetSecurityMiddleware())
}

func contextInitializer(ctx context.Context) fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestHeaders := map[string]interface{}{}
		c.Request().Header.VisitAll(func(key, value []byte) {
			requestHeaders[string(key)] = string(value)
		})

		requestCtx := ctxmanager.InitContext(ctx, requestHeaders)
		c.SetUserContext(requestCtx)
		return c.Next()
	}

}

func responseEnricher() fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := ctxhelper.AddResponsePropagatableContextData(c.UserContext(), c.Response().Header.Add)
		if err != nil {
			logger.ErrorC(c.UserContext(), "can't insert propagatable  data to incoming response. Error %+v", err)
			return err
		}
		return c.Next()
	}
}

func (builder *Builder) enableActuatorEndpoints(app *fiber.App) error {
	if builder.health != nil {
		app.Get(builder.health.url, healthpoint.EnableHealth(builder.health.healthService.Start()))
	}
	if builder.pprofPort != "" {
		pprofpoint.EnablePprofOnPort(builder.pprofPort)
	}
	if builder.prometheusURL != "" {
		err := monitorpoint.EnablePrometheus(builder.prometheusURL, &builder.prometheusConfig, app)
		if err != nil {
			return err
		}
	}
	if builder.exporter != nil {
		err := tracingpoint.EnableOtelTracing(builder.exporter, app)
		if err != nil {
			return err
		}
	}
	if builder.apiVersionService != nil {
		app.Get("/api-version", apiversionpoint.EnableApiVersion(builder.apiVersionService))
	}
	if builder.logLevelService != nil {
		app.Get("/api/logging/v1/levels", loglevelpoint.EnableLogLevel(builder.logLevelService))
	}
	return nil
}
