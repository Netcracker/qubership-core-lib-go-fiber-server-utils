---
name: fiber-server-utils-go-usage
description: Builder for fiber.App preconfigured with Qubership cloud-core middleware (context-propagation, security, request-id-aware logging, health, prometheus, pprof, tracing). Use when writing or reviewing Go microservices that expose REST over fiber.
---

# qubership-core-lib-go-fiber-server-utils

Wraps `github.com/gofiber/fiber/v2` with a builder that wires up
context propagation, security middleware, request-id-aware log
formatting, and optional health / prometheus / pprof / tracing
endpoints. Without it you have to wire each of these yourself
on every service.

## Why it matters

A bare `fiber.New()` does not:
- propagate `X-Request-Id`, `X-Version`, `Accept-Language` etc.
  through `ctx.UserContext()`
- inject `request_id` / `tenant_id` into log lines
- write `X-Request-Id` back into the HTTP response
- enforce platform security headers/JWT

`fiberserver.New().Process()` does all of the above in one call.

## Import

```go
import (
    "github.com/gofiber/fiber/v2"
    fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2"
    "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/security"
    "github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
)
```

## Required setup order (do not reorder)

1. `configloader.InitWithSourcesArray(configloader.BasePropertySources())`
2. `logging.GetLogger("main")` (depends on configloader)
3. `ctxmanager.Register(baseproviders.Get())`
4. `serviceloader.Register(1, &security.DummyFiberServerSecurityMiddleware{})`
   — REQUIRED. Without this `Process()` panics with
   `"can not find implementation for security.SecurityMiddleware"`.
   Use `DummyFiberServerSecurityMiddleware` for unauthenticated
   services; replace with a real implementation for prod.
5. `fiberserver.New(...).Process()` — only now.

## Builder API

```go
fiberserver.New(config ...fiber.Config) *Builder
```

| Method | Purpose |
|---|---|
| `WithHealth(path, svc)` | health endpoint backed by `health.HealthService` |
| `WithApiVersion(svcs...)` | `/api-version` endpoint |
| `WithPrometheus(path, ...)` | metrics endpoint, default counter + latency histogram |
| `WithPprof(port)` | pprof on `localhost:<port>` (not exposed outside the pod) |
| `WithTracer(exporter)` | OpenTelemetry tracing, B3 headers; usually `tracing.NewZipkinTracer()` |
| `WithLogLevelsInfo()` | `/api/logging/v1/levels` — current logger levels |
| `WithDeprecatedApiSwitchedOff()` | makes endpoints in `deprecated.api.patterns` return TMF 404 |
| `Process()` | builds `*fiber.App` |
| `ProcessWithContext(ctx)` | same, but bound to a context for graceful shutdown |

`Process` and `ProcessWithContext` return `(*fiber.App, error)` —
always check the error.

## Canonical service skeleton

```go
package main

import (
    "os"

    "github.com/gofiber/fiber/v2"
    fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2"
    "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/security"
    "github.com/netcracker/qubership-core-lib-go/v3/configloader"
    "github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders"
    "github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxmanager"
    "github.com/netcracker/qubership-core-lib-go/v3/logging"
    "github.com/netcracker/qubership-core-lib-go/v3/serviceloader"

    _ "github.com/netcracker/qubership-core-lib-go/v3/memlimit"
)

var logger logging.Logger

func init() {
    configloader.InitWithSourcesArray(configloader.BasePropertySources())
    logger = logging.GetLogger("main")
    ctxmanager.Register(baseproviders.Get())
    serviceloader.Register(1, &security.DummyFiberServerSecurityMiddleware{})
}

func main() {
    app, err := fiberserver.New().Process()
    if err != nil {
        logger.Error("fiber build failed: " + err.Error())
        os.Exit(1)
    }

    app.Get("/hello", helloHandler)

    if err := app.Listen(":8080"); err != nil {
        logger.Error("listen failed: " + err.Error())
        os.Exit(1)
    }
}

func helloHandler(c *fiber.Ctx) error {
    logger.InfoC(c.UserContext(), "Handling /hello request")
    return c.SendString("hello")
}
```

## Reading request-scoped context in a handler

`fiber.Ctx.UserContext()` returns the context populated by the
context-propagation middleware. Read providers off of it:

```go
import "github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/xrequestid"

func handler(c *fiber.Ctx) error {
    obj, err := xrequestid.Of(c.UserContext())
    if err != nil { /* not in context */ }
    id := obj.GetRequestId()
    ...
}
```

For logging, prefer `logger.InfoC(ctx, msg)` / `DebugC` / `WarnC` /
`ErrorC` — the context-aware variants populate the `request_id` /
`tenant_id` log fields automatically. Plain `logger.Info(msg)`
still works but logs `request_id=-`.

## Default fiber config overrides

`fiberserver.New()` overrides these unless you pass your own:

| Parameter | Default |
|---|---|
| `ReadBufferSize` | 8192 |

Fiber defaults to IPv4 only. For dual-stack:

```go
fiberserver.New(fiber.Config{Network: fiber.NetworkTCP})
```

## TMF error handling

Wire the platform error handler when building the app:

```go
import fibererrors "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v2/errors"

unknown := errs.ErrorCode{Code: "MY-SVC-0001", Title: "unexpected error"}
app, err := fiberserver.New(fiber.Config{
    ErrorHandler: fibererrors.DefaultErrorHandler(unknown),
}).Process()
```

Custom errors implementing `ErrCodeErr` get rendered as TMF
responses; if they implement `Handle(ctx *fiber.Ctx) error` that
method takes over response shaping.

The `0001` suffix is reserved for the catch-all "unknown error"
code per service/library.

## Disabling deprecated APIs

```go
fiberserver.New(...).WithDeprecatedApiSwitchedOff().Process()
```

```yaml
# application.yaml
deprecated:
  api:
    disabled: true
    patterns:
      - /api/v1/** [GET POST DELETE]
      - /api/v2/**
```

Override via `DEPRECATED_API_DISABLED` env (requires
`EnvPropertySource` in configloader). Matched routes return
TMF 404 with code `NC-COMMON-2101`.

## Anti-patterns

```go
// WRONG: bare fiber — no context propagation, no request_id in logs,
// no X-Request-Id in response, no security middleware.
app := fiber.New()

// WRONG: skipping security registration. Process() will panic.
app, _ := fiberserver.New().Process() // before serviceloader.Register(...)

// WRONG: ignoring the error from Process / ProcessWithContext.
app, _ := fiberserver.New().Process()

// WRONG: reading request data from c.Context() instead of c.UserContext().
// c.Context() is the fasthttp ctx — it does not carry the propagated values.
xrequestid.Of(c.Context())

// WRONG: plain logger.Info inside a handler — request_id will be "-".
logger.Info("handling")            // use logger.InfoC(c.UserContext(), ...)
```

## Verifying it works

```sh
$ curl -i -H "X-Request-Id: abc-123" http://localhost:8080/hello
HTTP/1.1 200 OK
X-Request-Id: abc-123          # propagated back into response
```

```
# log line
[INFO] [request_id=abc-123] [tenant_id=] [class=main] Handling /hello request
```

If the response is missing `X-Request-Id` or logs show
`request_id=-`, the middleware is not active — usually because
`Process()` was never called, or the handler used `logger.Info`
instead of `logger.InfoC(c.UserContext(), ...)`.
