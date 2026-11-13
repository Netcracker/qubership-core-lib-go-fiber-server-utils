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
- [Upgrade from v2](#upgrade-from-v2)
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

Server-utils v3 requires Go 1.26.5 or later and uses Fiber 3.7.0.
To install the Qubership library, replace `<server-utils-version>` with its tag, pseudo-version, or commit:

```shell
go get github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3@<server-utils-version>
```

List of all released versions may be found [here](https://github.com/netcracker/qubership-core-lib-go-fiber-server-utils/tags)

## Upgrade from v2

These instructions are for applications using `qubership-core-lib-go-fiber-server-utils/v2` with Fiber v2.
The upgrade changes two Go modules: the Qubership library moves to `/v3`, and the Fiber framework moves
to `github.com/gofiber/fiber/v3`. The Qubership library needs a new major version because its public
server, security, and error-handling APIs expose Fiber types.

### 1. Update dependencies and imports

Choose the v3 release, pseudo-version, or reviewed commit of the Qubership library that your application
will use. In each Go module that depends on server-utils, including separate test modules, run:

```shell
go get github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3@<server-utils-version>
```

Here `<server-utils-version>` is the version of the Qubership library, not the Fiber version.
Then update the Go import declarations in application code and tests:

- Replace the prefix `github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2` with
  `github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3`, including subpackages such as `security` and `errors`.
- Replace `github.com/gofiber/fiber/v2` with `github.com/gofiber/fiber/v3`.

### 2. Update handler and middleware signatures

Change the request parameter from `*fiber.Ctx` to `fiber.Ctx` in route handlers, security middleware,
callbacks, and custom error `Handle` methods. Fiber v3 defines `fiber.Ctx` as a Go interface.

In the examples below, `fiber` is the import name for `github.com/gofiber/fiber/v3`.
The parameter named `request` is the HTTP request context that Fiber passes to the handler:

```go
func handleRequest(request fiber.Ctx) error {
    return request.SendStatus(fiber.StatusOK)
}
```

### 3. Use the appropriate request context

Replace calls to the Fiber v2 `request.UserContext()` method with `request.Context()`.
This returns the application `context.Context` populated by server-utils with request and tenant data.
Pass it to logging and downstream operations that accept a Go context. If your code sets this context
explicitly, replace `request.SetUserContext(...)` with `request.SetContext(...)`.

Review existing calls to the Fiber v2 `request.Context()` method separately: that method returned
`*fasthttp.RequestCtx`. Its replacement is `request.RequestCtx()`, which is needed by fasthttp-based
integrations such as WebSocket upgrades or code reading the remote network address.

This middleware demonstrates both uses. The logger receives the application context, while
RemoteAddr reads the transport context:

```go
func logRequest(request fiber.Ctx) error {
    applicationContext := request.Context()
    transportContext := request.RequestCtx()
    logger := logging.GetLogger("http")
    logger.InfoC(applicationContext, "Remote address: %s", transportContext.RemoteAddr().String())
    return request.Next()
}
```

Do not retain the `fiber.Ctx` handler parameter after the handler returns: Fiber reuses that object.
Use the application context when passing request metadata to downstream operations.

### 4. Move listener settings to server startup

If your Fiber v2 application set `Network` or `DisableStartupMessage` in `fiber.Config`, remove those
fields from the application configuration. Pass their values to `server.StartServer` or
`server.StartServerOnAddress` through `fiber.ListenConfig` instead.

For example, an application that used `Network: fiber.NetworkTCP` and `DisableStartupMessage: true`
should start its server as follows. Here `app` is the `*fiber.App` returned by the builder's `Process`
method, and `server` is imported from `github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3/server`:

```go
server.StartServer(app, "http.server.bind", fiber.ListenConfig{
    ListenerNetwork:       fiber.NetworkTCP,
    DisableStartupMessage: true,
})
```

Keep the bind-address property used by your application; this example uses `http.server.bind`.
Preserve the old Network value: `tcp` permits IPv4 and IPv6, while the default `tcp4` is IPv4-only.
If the application used the default network, omit ListenerNetwork.

The server wrappers apply these listener options to both HTTP and platform TLS.
When platform TLS is enabled, certificates, CA, and client-auth settings still come from core-lib-go;
TLS fields in ListenConfig do not override that platform configuration.

### 5. Review request parsing and response compatibility

- In handlers that parse a body or query string, replace `request.BodyParser(&value)` with
  `request.Bind().Body(&value)` and `request.QueryParser(&value)` with `request.Bind().Query(&value)`.
  For path parameters, replace `request.ParamsParser(&value)` with `request.Bind().URI(&value)`
  and change struct tags such as `params:"id"` to `uri:"id"` so the fields are populated.
- If a handler constructs URLs or records request metadata, use `request.Scheme()` for `http` or `https`.
  Fiber v3 `request.Protocol()` returns the HTTP protocol version. Use `request.Host()` when the port
  must be included; `request.Hostname()` returns the hostname without the port.
- Review consumer handlers that call `request.JSON(value)`: Fiber v3 sets their Content-Type to
  `application/json; charset=utf-8`. If the existing HTTP contract requires `application/json`,
  call `request.JSON(value, fiber.MIMEApplicationJSON)`. JSON responses produced by server-utils
  already preserve the previous Content-Type.

See the [Fiber migration guide](https://docs.gofiber.io/whats_new/#migration-guide) for changes to
other Fiber APIs used directly by your application, including routing, middleware, adaptor, and utilities.

### 6. Update tests and verify the consumer

In tests that call `app.Test`, replace the Fiber v2 integer timeout with a `fiber.TestConfig`.
To preserve the no-timeout behavior of `app.Test(testRequest, -1)`, use
`app.Test(testRequest, fiber.TestConfig{Timeout: 0})`.
For a positive timeout such as `3000` milliseconds, use
`fiber.TestConfig{Timeout: 3 * time.Second, FailOnTimeout: true}` so a timeout still fails the test.

After updating the code, run these commands from each changed Go module:

```shell
go mod tidy
go list -m all
go build ./...
go vet ./...
go test ./...
```

Inspect the module list for dependencies that still use Fiber v2. Compare response statuses, headers,
and bodies with the v2 application, and run the consumer's context propagation, HTTP/TLS, and WebSocket
tests for the features it uses.

## Usage

At first, you need to initialize configloader with desired property sources. Each property source (for example application.yaml, env variables or config-server)
defines a source from which your application will receive properties.

```go
configloader.Init(configloader.BasePropertySources()...) // to use YAML and ENV property sources
```
If you want to use `config-server` as property source you have to add extra dependency: [rest-utils](https://github.com/Netcracker/qubership-core-lib-go-rest-utils) module.
In this case configuration loading should be initialized as
```go
configloader.Init(configserver.AddConfigServerPropertySource(configloader.BasePropertySources())...)
```

More information about property sources' configuration may be found at [configloader](https://github.com/netcracker/qubership-core-lib-go/blob/main/configloader/README.md)
and at [config-server](https://github.com/Netcracker/qubership-core-lib-go-rest-utils/blob/main/configserver-propertysource/README.md)

> **_NOTE:_**  Configuration loading should be performed as one of the first steps in bootstrap time. 
>That is why we strongly recommend putting configloader#Init method as the first position in the main#init method (see [quick example](#quick-example)).

After properties initialization you should register security implemention - dummy or your own, the followning example shows registration of required services:

```go
import (
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
  "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3/security"
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

| **Parameter name** | **Default value** | **Configuration property**    |
|--------------------|-------------------|-------------------------------|
| ReadBufferSize     | 10240 bytes       | `http.buffer.header.max.size` |

The builder reads this property when ReadBufferSize is omitted or nonpositive.
A positive ReadBufferSize supplied in `fiber.Config` takes precedence over the property.

#### New

Method `fiberserver.New(config ...fiber.Config)` creates new builder instance. You may pass optional settings `fiber.Config` as a parameter 
for `fiber.App` configuration or don't pass anything and use default config.

```go
config := fiber.Config{
    CaseSensitive: true,
    ServerHeader:  "Fiber",
    AppName: "Test App v1.0.1",
}
app, err := fiberserver.New(config).Process()
```

> **_NOTE:_** The default listener is IPv4-only. Use
> `fiber.ListenConfig{ListenerNetwork: fiber.NetworkTCP}` when starting the server for IPv4 and IPv6.


#### WithHealth
Method `WithHealth(url string, healthService health.HealthService)` allows adding health monitoring to service for processing health indicators and showing health state. 
* Param `path`, _string_ is a path to the future health endpoint. For example `"/health"`.
* Param `HealthService`, service which processes and renders health information. 
How to create _health.HealthService_, configure it, and add health indicators can be found on this page: 
[go-actuator](https://github.com/netcracker/qubership-core-lib-go-actuator-common/blob/main/README.md#health-core).

enabling health usage:  
```go
healthService, err := health.NewHealthService()
app, err := fiberserver.New().
	WithHealth("/health", healthService).
	Process()
```


#### WithApiVersion
Method `WithApiVersion(apiVersionServices ...apiversion.ApiVersionService)` allows adding api-version endpoint to service for checking version of api.
* Param `apiVersionServices` allows to set service with custom implementation. If this parameter is empty, default apiVersionService will be provided.
  How to create _apiversion.ApiVersionService_ and configure it can be found on this page:
  [go-actuator](https://github.com/netcracker/qubership-core-lib-go-actuator-common/blob/main/README.md#api-version).

enabling api-version usage:
```go
apiVersionService, err := apiversion.NewApiVersionService(apiversion.ApiVersionConfig{PathToApiVersionInfoFile: "./testdata/api-version-info.json"})
app, err := fiberserver.New().
	WithApiVersion(apiVersionService).
	Process()
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
app, err := fiberserver.New().
	WithPrometheus("/prometheus").
	Process()
```

#### WithPprof
Method `WithPprof(port string)` allows starting `pprof` on `localhost` on the desired port. Pprof is a tool that is designed by Golang and
is intended for visualization and analysis of profiling data.
* Param `port`, _string_ is a port to the future pprof. For example `"6060"`.

> **_NOTE:_**  Pprof is enabled on `localhost` and not available out of the pod.

enabling pprof usage:
```go
app, err := fiberserver.New().
	WithPprof("6060").
	Process()
```

#### WithTracer
Method `WithTracer(exporter OpenTelemetryExporter, opts ...TracerOption)` adds tracing to service. Tracer allows to trace requests. Supports `B3` headers.
* Param `exporter`, _tracing.OpenTelemetryExporter_ is an interface that returns tracing provider. Out of the box, we provide `Zipkin tracer` from `qubership-core-lib-go-actuator-common`.
* Optional `SkipTracing("/path", ...)` marks additional exact paths that must not create spans (for example service-owned probes registered outside the builder).

##### Skipping probe and actuator paths (why it lives here)

Previously `RateLimitingSampler` in `actuator-common` dropped some HTTP paths inside the OpenTelemetry sampler (`/health`, `/static` prefix, and later a hardcoded list in a path-filtering sampler experiment). That approach had several problems:

* The skip list could diverge from routes actually registered by the Fiber builder (extra k8s probe paths, missing `/api/logging/v1/levels`, and so on).
* Filtering happened **after** the middleware had already built span attributes (`ParseIP`, headers, and so on), so excluded requests still paid that cost.
* Using `Drop` in the sampler can break an already-sampled parent trace (B3); skipping in middleware simply does not create a server span and leaves the incoming context unchanged.
* Zipkin/sampler configuration is the wrong extension point: span creation happens in this library's `tracingpoint` middleware, not in the exporter.

Path filtering was therefore **removed from actuator-common**. That library now only registers the Zipkin exporter and applies the rate limit in `RateLimitingSampler`. All path-based exclusions are handled here, in Fiber middleware, **before** `tracer.Start(...)`.

##### How span skipping works now

When `Process()` runs, the builder:

1. Collects an `untraced` map (exact paths only).
2. Registers the OTel tracing middleware first (so later routes pass through it).
3. Adds each enabled actuator route to `untraced` at registration time (`registerUntraced` or an explicit map entry for Prometheus).

On each request the middleware calls `shouldSkipTracing`:

* **Exact path** — request path (`c.Path()`) or matched route template (`c.Route().Path`) is in `untraced` → `return c.Next()` with no span.
* **Prefix** — path starts with `/static` (platform default carried over from the old sampler behaviour) → no span.
* Otherwise a server span is created, exported via the Zipkin tracer from actuator-common, and subject only to **rate limiting** in the sampler (default 10 traces/sec).

Paths skipped automatically when the corresponding builder method is used:

| Builder | Path added to skip list |
|---|---|
| `WithHealth(url, ...)` | `url` |
| `WithPrometheus(url, ...)` | `url` |
| `WithApiVersion()` | `/api-version` |
| `WithLogLevelsInfo()` | `/api/logging/v1/levels` |
| (always) | any path with prefix `/static` |

Service-specific probes that you register on the returned `fiber.App` after `Process()` (for example `app.Get("/ready", ...)`) are **not** added automatically — pass them via `SkipTracing`.

There are two ways to initiate `zipkinTracer`:
* Use `tracing.NewZipkinTracer()` to use configuration with environment parameters. 
* Use `tracing.NewZipkinTracerWithOpts(zipkinOptions ZipkinOptions)` to pass parameters directly.

Table below determines env parameters which should be set during configuration with `tracing.NewZipkinTracer()`. If user
chooses configuration with `tracing.NewZipkinTracerWithOpts(zipkinOptions ZipkinOptions)` all environment properties would be ignored.

|Name|Description|Default|Allowed values|
|---|---|---|---|
|tracing.enabled  | Enable or disable tracing (to switch on/off without changing other params) | false | true/false|
|tracing.host     | Zipkin host server, without port and protocol | -- | any string, for example nc-diagnostic-agent
|tracing.sampler.ratelimiting | max traces per second (rate limiting sampler) | 10 | positive integer |
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
app, err := fiberserver.New().
    WithTracer(tracing.NewZipkinTracer()).
	Process() // build fiber.App with zipkin tracer enabled
```

Skip custom probes:
```go
app, err := fiberserver.New().
    WithTracer(tracing.NewZipkinTracer(), fiberserver.SkipTracing("/health", "/ready")).
	Process()
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
app, err := fiberserver.New().
    WithTracer(tracing.NewZipkinTracerWithOpts(options)).
	Process() // build fiber.App with zipkin tracer enabled
``` 

#### WithLogLevelsInfo
Method WithLogLevelsInfo() allows adding log levels info endpoint to service for getting currently used log levels for all created loggers.

Enabling log levels info usage:
```go
app, err := fiberserver.New().
	WithLogLevelsInfo().
	Process()
```

The endpoint will be opened on `/api/logging/v1/levels` path.

#### Process
Method `Process()` builds configured `fiber.App` instance for future work. This method also registers base and security context providers and enables security auth.

#### ProcessWithContext
Method `ProcessWithContext(ctx context.Context)` uses the supplied context as the parent for request contexts.
Canceling it cancels downstream operations that use those contexts. To stop the listener as well,
pass it in `fiber.ListenConfig.GracefulContext` when starting the server, or call `app.Shutdown()`.

```go
ctx, cancel := context.WithCancel(context.Background())
app, err := fiberserver.New().ProcessWithContext(ctx)
cancel()
``` 

### How to obtain context object
`context.Context` object has request scope and contains request scope context data. So, during each request, we initialize `context.Context` object and fill in it data 
based on registered contexts and request data. List of registered contexts can be found [here](https://github.com/netcracker/qubership-core-lib-go/blob/main/context-propagation/README.md#base-contexts) 
 

Call `fiber.Ctx.Context()` to obtain the populated application context in a handler. For example:
```go
app.Get("/test", func(ctx fiber.Ctx) error {
       requestID, err := xrequestid.Of(ctx.Context())
       if err != nil {
           return err
       }
       return ctx.SendString(requestID.GetRequestId())
})

```

### Default error handling
To comply to REST response in ErrorCode format the following default error handling mechanism is provided via fiber-server-utils/errors/default.go
To configure your fiber server to provide error response in NC specific TMF format the following code must be written:

```go
import 	(
  fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3"
  fibererrors "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3/errors"
)
// code for all unexpected errors for your microservice/lib must start with corresponding abbreviation followed by digital code 0001 
// 0001 digital part of the code is reserved specifically for this purpose
unknownErrorCode := errs.ErrorCode{Code: "YOUR-MS_OR_LIB-0001", Title: "unexpected error"}
app, err := fiberserver.New(fiber.Config{
    IdleTimeout:  30 * time.Second,
    ErrorHandler: fibererrors.DefaultErrorHandler(unknownErrorCode),
}).Process()
```

This handler 
1) allows to process all errors of type error as unknown errors.
2) provides default handler for all errors which implement github.com/netcracker/qubership-core-lib-go-error-handling/errors.ErrCodeErr interface
3) provides delegation of handling particular error to its own method - func(ctx fiber.Ctx) error. See example below:
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

   func (e *CustomErr) getMeta(ctx fiber.Ctx) map[string]any {
    return map[string]any{ "custom": e.CustomField }
   }
   
   func (e *CustomErr) Handle(ctx fiber.Ctx) error {
     status := http.StatusBadRequest
     response := tmf.NewResponseBuilder(e).
		Meta(e.getMeta(ctx)).
        Status(status).
		Build()
	 return ctx.Status(status).JSON(response, fiber.MIMEApplicationJSON)
   }
   ```

### Disable deprecated API
This library allows to disable REST API in a Fiber go microservice. This allows to make deprecated REST endpoints return
TMF error responses with 404 HTTP status code and predefined error code NC-COMMON-2101 as if endpoint has been already removed.
Deprecated REST API is the set of REST endpoints specified as ant path patterns via property 'deprecated.api.patterns' 
provided in application.yaml file.

```go
import 	(
  fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3"
)
app, err := fiberserver.New(fiber.Config{
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
  "github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
  fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3"
  "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3/security"
  "github.com/netcracker/qubership-core-lib-go-actuator-common/v2/health"
  "github.com/netcracker/qubership-core-lib-go-actuator-common/v2/tracing"
  "github.com/gofiber/fiber/v3"
)

var logger logging.Logger

func init() {
  // provide property sources
  configloader.Init(configloader.BasePropertySources()...)
  logger = logging.GetLogger("main")
  serviceloader.Register(1, &security.DummyFiberServerSecurityMiddleware{})
}

func main() {
  healthService, err := health.NewHealthService()
  if err != nil {
    logger.Error("Cannot create health service: %v", err)
    return
  }
  app, err := fiberserver.New().
    WithPprof("6060").
    WithPrometheus("/prometheus").
    WithHealth("/health", healthService).
    WithTracer(tracing.NewZipkinTracer()).
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

func fooHandler(ctx fiber.Ctx) error  {
  return nil
}

func customHandler(ctx fiber.Ctx) error  {
  return nil
}
```

