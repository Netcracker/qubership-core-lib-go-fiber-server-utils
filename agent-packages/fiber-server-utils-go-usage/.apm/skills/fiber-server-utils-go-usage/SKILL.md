---
name: fiber-server-utils-go-usage
description: Use when wiring a Fiber HTTP server in a Qubership Go microservice with platform middleware (context propagation, request-id logging, security).
---

# qubership-core-lib-go-fiber-server-utils

Wraps `github.com/gofiber/fiber/v2` with a builder that wires up
context propagation, security middleware, request-id-aware log
formatting, and optional health / prometheus / pprof / tracing
endpoints. Without it you have to wire each of these yourself
on every service.

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

## Common pitfalls

- `c.Context()` instead of `c.UserContext()` — `c.Context()` is the fasthttp ctx and does not carry propagated values; read providers via `c.UserContext()`.
- `logger.Info(msg)` inside a handler — logs `request_id=-`. Use `logger.InfoC(c.UserContext(), msg)` (also `DebugC` / `WarnC` / `ErrorC`).
- Bare `fiber.New()` — bypasses context propagation, request-id logging, response header injection, and security middleware. Use `fiberserver.New(...).Process()`.
- Ignoring the `(*fiber.App, error)` from `Process` / `ProcessWithContext` — masks build-time wiring errors.
- Calling `Process()` before `serviceloader.Register(1, &security.DummyFiberServerSecurityMiddleware{})` — panics with `"can not find implementation for security.SecurityMiddleware"`.

