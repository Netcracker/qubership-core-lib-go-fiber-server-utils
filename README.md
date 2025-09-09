[![Go build](https://github.com/Netcracker/qubership-core-lib-go-fiber-server-utils/actions/workflows/go-build.yml/badge.svg)](https://github.com/Netcracker/qubership-core-lib-go-fiber-server-utils/actions/workflows/go-build.yml)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?metric=coverage&project=Netcracker_qubership-core-lib-go-fiber-server-utils)](https://sonarcloud.io/summary/overall?id=Netcracker_qubership-core-lib-go-fiber-server-utils)
[![duplicated_lines_density](https://sonarcloud.io/api/project_badges/measure?metric=duplicated_lines_density&project=Netcracker_qubership-core-lib-go-fiber-server-utils)](https://sonarcloud.io/summary/overall?id=Netcracker_qubership-core-lib-go-fiber-server-utils)
[![vulnerabilities](https://sonarcloud.io/api/project_badges/measure?metric=vulnerabilities&project=Netcracker_qubership-core-lib-go-fiber-server-utils)](https://sonarcloud.io/summary/overall?id=Netcracker_qubership-core-lib-go-fiber-server-utils)
[![bugs](https://sonarcloud.io/api/project_badges/measure?metric=bugs&project=Netcracker_qubership-core-lib-go-fiber-server-utils)](https://sonarcloud.io/summary/overall?id=Netcracker_qubership-core-lib-go-fiber-server-utils)
[![code_smells](https://sonarcloud.io/api/project_badges/measure?metric=code_smells&project=Netcracker_qubership-core-lib-go-fiber-server-utils)](https://sonarcloud.io/summary/overall?id=Netcracker_qubership-core-lib-go-fiber-server-utils)

# Fiber-server-utils

`Fiber server utils` allows enabling some basic cloud-core functionality (such as security, registering context providers, etc.) 
and create `fiber.App` instance that has all necessary configuration for correct work in the cloud-core environment.

- [Install](#install)
- [Usage](#usage)
  * [Builder](#builder)
      * [Overridden configs](#overridden-configs)
      * [New](#new)
      * [WithHealth](#withhealth)
      * [WithApiVersion](#withapiversion)
      * [WithPrometheus](#withprometheus)
      * [WithPprof](#withpprof)
      * [WithTracer](#withtracer)
      * [WithLogLevelsInfo](#withloglevelsinfo)
      * [Process](#process)
  * [Policy.conf](#how-to-create-policyconf)
  * [How to obtain context object](#how-to-obtain-context-object)
  * [Default error handling](#default-error-handling)
  * [Disable deprecated API](#disable-deprecated-api)
- [Quick example](#quick-example)

## Install

To get `fiber-server-utils` use
```go
 go get github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2@<latest released version>
```

List of all released versions may be found [here](https://github.com/netcracker/qubership-core-lib-go-fiber-server-utils/tags)

## Usage

At first, you need to initialize configloader with desired property sources. Each property source (for example application.yaml, env variables or config-server)
defines a source from which your application will receive properties.

```go
configloader.Init(configloader.BasePropertySources()) // to use YAML and ENV property sources
```
If you want to use `config-server` as property source you have to add extra dependency: [rest-utils](https://github.com/Netcracker/qubership-core-lib-go-rest-utils) module.
In this case configuration loading should be initialized as
```go
configloader.Init(configserver.AddConfigServerPropertySource(configloader.BasePropertySources())) 
```

More information about property sources' configuration may be found at [configloader](https://github.com/netcracker/qubership-core-lib-go/blob/main/configloader/README.md)
and at [config-server](https://github.com/Netcracker/qubership-core-lib-go-rest-utils/blob/main/configserver-propertysource/README.md)

> **_NOTE:_**  Configuration loading should be performed as one of the first steps in bootstrap time. 
>That is why we strongly recommend putting configloader#Init method as the first position in the main#init method (see [quick example](#quick-example)).

After properties initialization you should register security implemention - dummy or your own, the followning example shows registration of required services:

```go
import (
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
  "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/security"
)

func init() {
  serviceloader.Register(1, &security.DummyFiberServerSecurityMiddleware{})
}
```

After that you may start creating `fiber.App` with provided builder.

### Builder
`fiber-server-utils` uses the builder pattern and allows a convenient way to configure and build `fiber.App` instance. 
Builder has methods that allow adding health check, monitoring metrics, pprof monitoring and tracing. 
Additionally, it configures logger' message format (print context x_request_id and tenantId), enable context propagation and security auth.
All these things are needed for correct operation in a cloud.

#### Overridden configs
Configuration settings of fiber server will be overridden with default values 
unless you explicitly specify them in the configuration object which provided to method [New()](#new).

A table with default values is shown below:

| **Parameter name** | **Default value** | **Status**          |
|--------------------|-------------------|---------------------|
| ReadBufferSize     | 8192              | since version 0.5.1 |

#### New

Method `fiberserver.New(config ...fiber.Config)` creates new builder instance. You may pass optional settings `fiber.Config` as a parameter 
for `fiber.App` configuration or don't pass anything and use default config.

```go
config := fiber.Config{
    CaseSensitive: true,
    ServerHeader:  "Fiber",
    AppName: "Test App v1.0.1"
})
app := fiberserver.New(config).Process() // build fiber.App without any cloud-core specific configuration
```

> **_NOTE:_**  Default fiber configuration allows working only with IPv4.
> If you need to use IPv4 and IPv6, add such configuration to app builder initialization
> 
> appBuilder, err := fiberserver.New(fiber.Config{Network: fiber.NetworkTCP})


#### WithHealth
Method `WithHealth(url string, healthService health.HealthService)` allows adding health monitoring to service for processing health indicators and showing health state. 
* Param `path`, _string_ is a path to the future health endpoint. For example `"/health"`.
* Param `HealthService`, service which processes and renders health information. 
How to create _health.HealthService_, configure it, and add health indicators can be found on this page: 
[go-actuator](https://github.com/netcracker/qubership-core-lib-go-actuator-common/blob/main/README.md#health-core).

enabling health usage:  
```go
app := fiberserver.New()
	.WithHealth("/health", health.NewHealthService(time.Health)) // existing implementation from go-actuator
	.Process() // build fiber.App with health enabled
```


#### WithApiVersion
Method `WithApiVersion(apiVersionServices ...*apiversion.ApiVersionService)` allows adding api-version endpoint to service for checking version of api.
* Param `apiVersionServices` allows to set service with custom implementation. If this parameter is empty, default apiVersionService will be provided.
  How to create _apiversion.ApiVersionService_ and configure it can be found on this page:
  [go-actuator](https://github.com/netcracker/qubership-core-lib-go-actuator-common/blob/main/README.md#api-version).

enabling api-version usage:
```go
apiVersionService, err := apiversion.NewApiVersionService(apiversion.ApiVersionConfig{PathToApiVersionInfoFile: "./testdata/api-version-info.json"})
app := fiberserver.New()
	.WithApiVersion(&apiVersionService)
	.Process() // build fiber.App with api-version enabled
```

#### WithPrometheus
Method `WithPrometheus(path string)` adds ability to collect server prometheus metrics.
* Param `path`, _string_. E.g.: `"/prometheus"`. By this path you can get Prometheus metrics.  

By default the following metrics are registered:
  * request counter;
  * request latency histogram.

We use global default prometheus instance. So, for your customization, for example: adding or removing metrics, 
you can use this default global instance too. 

enabling prometheus usage:
```go
app := fiberserver.New()
	.WithPrometheus("/prometheus") 
	.Process() // build fiber.App with prometheus metrics enabled
```

#### WithPprof
Method `WithPprof(port string)` allows starting `pprof` on `localhost` on the desired port. Pprof is a tool that is designed by Golang and
is intended for visualization and analysis of profiling data.
* Param `port`, _string_ is a port to the future pprof. For example `"6060"`.

> **_NOTE:_**  Pprof is enabled on `localhost` and not available out of the pod.

enabling pprof usage:
```go
app := fiberserver.New()
	.WithPprof("6060") 
	.Process() // build fiber.App with pprof enabled on localhost:6060
```

#### WithTracer
Method `WithTracer(exporter OpenTelemetryExporter)` adds tracing to service. Tracer allows to trace requests. Supports `B3` headers.
* Param `exporter`, _tracing.OpenTelemetryExporter_ is an interface that returns tracing provider. Out of the box, we provide `Zipkin tracer`.

There are two ways to initiate `zipkinTracer`:
* Use `tracing.NewZipkinTracer()` to use configuration with environment parameters. 
* Use `tracing.NewZipkinTracerWithOpts(zipkinOptions ZipkinOptions)` to pass parameters directly.

Table below determines env parameters which should be set during configuration with `tracing.NewZipkinTracer()`. If user
chooses configuration with `tracing.NewZipkinTracerWithOpts(zipkinOptions ZipkinOptions)` all environment properties would be ignored.

|Name|Description|Default|Allowed values|
|---|---|---|---|
|tracing.enabled  | Enable or disable tracing (to switch on/off without changing other params) | false | true/false|
|tracing.host     | Zipkin host server, without port and protocol | -- | any string, for example nc-diagnostic-agent
|tracing.sampler.const    | sampler always makes the same decision for all traces. It either samples all traces (value=1) or none of them (value=0). | 1 | 0 or 1
|microservice.name    | microservice name | -- | any string, for example tenant-manager

> **_NOTE:_**  If you set tracing.enabled=true but leave tracing.host empty, then you'll get an error. Also, if you'll specify tracing.host=some value,
> but leave tracing.enabled=false, tracing just won't be enabled.

enabling tracing usage:

With environment configuration:
```
MICROERVICE_NAME=service-name
TRACING_ENABLED=true
TRACING_HOST=nc-diagnostic-agent
```
```go
app := fiberserver.New()
    .WithTracer(tracing.NewZipkinTracer())
	.Process() // build fiber.App with zipkin tracer enabled
```

With direct configuration:
```go
options := tracing.ZipkinOptions{
		TracingEnabled:             true,
		TracingHost:                tracerHost,
		TracingSamplerRateLimiting: 10,
		ServiceName:                "service-name",
		Namespace:                  "namespace",
}
app := fiberserver.New()
    .WithTracer(tracing.NewZipkinTracerWithOpts(options))
	.Process() // build fiber.App with zipkin tracer enabled
``` 

#### WithLogLevelsInfo
Method WithLogLevelsInfo() allows adding log levels info endpoint to service for getting currently used log levels for all created loggers.

Enabling log levels info usage:
```go
app := fiberserver.New()
    .WithLogLevelsInfo()
	.Process() // build fiber.App with log levels info enabled
```

The endpoint will be opened on `/api/logging/v1/levels` path.

#### Process
Method `Process()` builds configured `fiber.App` instance for future work. This method also registers base and security context providers and enables security auth.

#### ProcessWithContext
Method `ProcessWithContext(ctx context.Context)` builds configured `fiber.App` with provided context. This context can be used for graceful shutdown in the future.

```go
ctx, cancel := context.WithCancel(context.Background())
app := fiberserver.New()
	.ProcessWithContext(ctx) // build fiber.App with provided context
...	
cancel() // graceful shutdown
``` 

### How to obtain context object
`context.Context` object has request scope and contains request scope context data. So, during each request, we initialize `context.Context` object and fill in it data 
based on registered contexts and request data. List of registered contexts can be found [here](https://github.com/netcracker/qubership-core-lib-go/blob/main/context-propagation/README.md#base-contexts) 
 

In order to obtain populated `context.Context` object, you should call `fiber.Ctx#UserContext` method in your handler. For example:
```go
app.Get("/test", func(ctx *fiber.Ctx) error {
       ctx := ctx.UserContext()
       requestId, err := xrequestid.Of(ctx)
       ...
})

```

### Default error handling
To comply to REST response in ErrorCode format the following default error handling mechanism is provided via fiber-server-utils/errors/default.go
To configure your fiber server to provide error response in NC specific TMF format the following code must be written:

```go
import 	(
  fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2"
  fibererrors "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/errors"
)
// code for all unexpected errors for your microservice/lib must start with corresponding abbreviation followed by digital code 0001 
// 0001 digital part of the code is reserved specifically for this purpose
unknownErrorCode := errs.ErrorCode{Code: "YOUR-MS_OR_LIB-0001", Title: "unexpected error"}
app, err := fiberserver.New(fiber.Config{
    Network:      fiber.NetworkTCP,
    IdleTimeout:  30 * time.Second,
    ErrorHandler: fibererrors.DefaultErrorHandler(unknownErrorCode),
}).Process()
```

This handler 
1) allows to process all errors of type error as unknown errors.
2) provides default handler for all errors which implement github.com/netcracker/qubership-core-lib-go-error-handling/errors.ErrCodeErr interface
3) provides delegation of handling particular error to its own method - func(ctx *fiber.Ctx) error. See example below:
   ```go
   import 	(
     errs "github.com/netcracker/qubership-core-lib-go-error-handling/v3/errors"
     tmf "github.com/netcracker/qubership-core-lib-go-error-handling/v3/tmf"
   )
   
   type CustomErr struct {
    *errs.ErrCodeError
    CustomField string
   }
   
   func NewCustomErr(detail string) *CustomErr {
    return errs.New(CustomErr{CustomField: detail}, errs.ErrorCode{Code:  "custom test error", Title: "custom test title",}, detail)
   }

   func (e *CustomErr) getMeta(ctx *fiber.Ctx) map[string]any {
    return map[string]any{ "custom": e.CustomField }
   }
   
   func (e *CustomErr) Handle(ctx *fiber.Ctx) error {
     status := http.StatusBadRequest
     response := tmf.NewResponseBuilder(e).
		Meta(e.getMeta()).
        Status(status).
		Build()
	 return ctx.Status(status).JSON(response)
   }
   ```

### Disable deprecated API
This library allows to disable REST API in a Fiber go microservice. This allows to make deprecated REST endpoints return
TMF error responses with 404 HTTP status code and predefined error code NC-COMMON-2101 as if endpoint has been already removed.
Deprecated REST API is the set of REST endpoints specified as ant path patterns via property 'deprecated.api.patterns' 
provided in application.yaml file.

```go
import 	(
  fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/"
  "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/deprecatedapi"
)
app, err := fiberserver.New(fiber.Config{
    Network:      fiber.NetworkTCP,
    IdleTimeout:  30 * time.Second,
// enable DeprecatedApiSwitchedOff feature. Deprecated API will be switched off only when property 'deprecated.api.disabled' = true
}).WithDeprecatedApiSwitchedOff().Process()
```

application.yaml configuration example:

```yaml
deprecated:
  api:
    disabled: true
    patterns:
      - /api/v1/** [GET POST DELETE]
      - /api/v2/**
```

to override 'deprecated.api.disabled' property from application.yaml, provide env DEPRECATED_API_DISABLED and 
make sure your configloader is using EnvPropertySource source
```go
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
```

#### 404 TMF response example:
```json
{
  "id": "13729bf0-fc38-4df0-a538-60a67956aa30",
  "code": "NC-COMMON-2101",
  "reason": "Request is declined with 404 Not Found, because deprecated REST API is disabled",
  "message": "Request [GET] '/deprecated-api/v2/test' is declined with 404 Not Found, because the following deprecated REST API is disabled: [[*]] /deprecated-api/v2/**",
  "status": "404",
  "@type": "NC.TMFErrorResponse.v1.0"
}
```

## Quick example

```go
package main

import (
  "github.com/netcracker/qubership-core-lib-go/v3/configloader"
  "github.com/netcracker/qubership-core-lib-go/v3/logging"
  fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2"
  "github.com/netcracker/qubership-core-lib-go-actuator-common/v2/health"
  "github.com/netcracker/qubership-core-lib-go-actuator-common/v2/tracing"
  "github.com/gofiber/fiber/v2"
  "time"
)

var logger logging.Logger

func init() {
  // provide property sources
  configloader.Init(configloader.BasePropertySources())
  logger = logging.GetLogger("main")
}

func main() {
  app, err := fiberserver.New().
    WithPprof("6060").
    WithPrometheus("/prometheus").
    WithHealth("/health", health.NewHealthService(time.Hour)).
    WithTracer(tracing.NewZipkinTracer(tracing.ZipkinOptions{ZipkinURL: "http://localhost:9411/api/v2/spans", ServiceName: "service-name", Namespace: "namespace"})).
    Process()
  if err != nil {
    logger.Error("Error during fiber app creation")
    return
  }

  // own microservice logic
  app.Get("/public-resource", customHandler)
  app.Get("/foo", fooHandler)

  app.Listen(":8080")
}

func fooHandler(ctx *fiber.Ctx) error  {
  userCtx := ctx.UserContext()
  // ...
  return nil
}

func customHandler(ctx *fiber.Ctx) error  {
  userCtx := ctx.UserContext()
  // ...
  return nil
}
```
