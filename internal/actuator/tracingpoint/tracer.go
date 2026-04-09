package tracingpoint

import (
	"context"
	"net"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go-actuator-common/v2/tracing"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	trace "go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "qubership.org/otelfiber"
)

var (
	logger       logging.Logger
	b3Propagator = b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))
)

func init() {
	logger = logging.GetLogger("fibertracing")
}

func EnableOtelTracing(exporter tracing.OpenTelemetryExporter, app *fiber.App) error {
	logger.Debug("Going to enable otel tracing")
	isRegistered, err := exporter.RegisterTracerProvider()
	if err != nil {
		logger.Error("Got error during tracer provider registry")
		return err
	}
	if isRegistered {
		app.Use(NewOtelTracingMiddleware(exporter.ServerName()))
	}
	return nil
}

func NewOtelTracingMiddleware(serverName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		spanOptions := concatSpanStartOptions([]trace.SpanStartOption{
			trace.WithAttributes(semconv.HTTPMethodKey.String(c.Method())),
			trace.WithAttributes(semconv.HTTPTargetKey.String(string(c.Request().RequestURI()))),
			trace.WithAttributes(semconv.HTTPRouteKey.String(c.Route().Path)),
			trace.WithAttributes(semconv.HTTPURLKey.String(c.OriginalURL())),
			trace.WithAttributes(semconv.HTTPUserAgentKey.String(string(c.Request().Header.UserAgent()))),
			trace.WithAttributes(semconv.HTTPRequestContentLengthKey.Int(c.Request().Header.ContentLength())),
			trace.WithAttributes(semconv.HTTPSchemeKey.String(c.Protocol())),
			trace.WithAttributes(semconv.HTTPServerNameKey.String(serverName)),
			trace.WithSpanKind(trace.SpanKindServer),
		}, remoteServerAttributes(c), hostAttributes(c))

		uCtx := extractB3HeadersToContext(c.UserContext(), c.Request())
		t := otel.GetTracerProvider().Tracer(tracerName)
		otelCtx, span := t.Start(
			uCtx,
			c.Route().Path,
			spanOptions...,
		)

		c.SetUserContext(otelCtx)
		defer span.End()

		err := c.Next()

		statusCode := c.Response().StatusCode()
		attrs := semconv.HTTPAttributesFromHTTPStatusCode(statusCode)
		spanStatus, spanMessage := semconv.SpanStatusFromHTTPStatusCode(statusCode)
		span.SetAttributes(attrs...)
		span.SetStatus(spanStatus, spanMessage)

		return err
	}
}

func hostAttributes(c *fiber.Ctx) []trace.SpanStartOption {
	logger.Debug("Start creating host attributes")
	options := []trace.SpanStartOption{}
	hostIP, hostName, hostPort := "", "", 0
	for _, someHost := range []string{c.Hostname(), string(c.Request().Header.Peek("Host")), string(c.Request().Host())} {
		hostPart := ""
		if idx := strings.LastIndex(someHost, ":"); idx >= 0 {
			strPort := someHost[idx+1:]
			numPort, err := strconv.ParseUint(strPort, 10, 16)
			if err == nil {
				hostPort = (int)(numPort)
			}
			hostPart = someHost[:idx]
		} else {
			hostPart = someHost
		}
		if hostPart != "" {
			ip := net.ParseIP(hostPart)
			if ip != nil {
				hostIP = ip.String()
			} else {
				hostName = hostPart
			}
			break
		} else {
			hostPort = 0
		}
	}
	if hostIP != "" {
		options = append(options, trace.WithAttributes(semconv.NetHostIPKey.String(hostIP)))
	}
	if hostName != "" {
		options = append(options, trace.WithAttributes(semconv.NetHostNameKey.String(hostName)))
	}
	if hostPort != 0 {
		options = append(options, trace.WithAttributes(semconv.NetHostPortKey.Int(hostPort)))
	}
	logger.Debugf("Formed host attributes: %+v", options)
	return options
}

func remoteServerAttributes(c *fiber.Ctx) []trace.SpanStartOption {
	logger.Debug("Start creating remotes server attributes")
	options := []trace.SpanStartOption{}
	peerName, peerIP, peerPort := "", "", 0
	{
		hostPart := c.Context().RemoteAddr().String()
		portPart := ""
		if idx := strings.LastIndex(hostPart, ":"); idx >= 0 {
			hostPart = c.Context().RemoteAddr().String()[:idx]
			portPart = c.Context().RemoteAddr().String()[idx+1:]
		}
		if hostPart != "" {
			if ip := net.ParseIP(hostPart); ip != nil {
				peerIP = ip.String()
			} else {
				peerName = hostPart
			}

			if portPart != "" {
				numPort, err := strconv.ParseUint(portPart, 10, 16)
				if err == nil {
					peerPort = (int)(numPort)
				} else {
					peerName, peerIP = "", ""
				}
			}
		}
	}
	if peerName != "" {
		options = append(options, trace.WithAttributes(semconv.NetPeerNameKey.String(peerName)))
	}
	if peerIP != "" {
		options = append(options, trace.WithAttributes(semconv.NetPeerIPKey.String(peerIP)))
	}
	if peerPort != 0 {
		options = append(options, trace.WithAttributes(semconv.NetPeerPortKey.Int(peerPort)))
	}
	logger.Debugf("Formed remote server attributes: %+v", options)
	return options
}

func concatSpanStartOptions(sources ...[]trace.SpanStartOption) []trace.SpanStartOption {
	var spanOptions []trace.SpanStartOption
	for _, source := range sources {
		for _, option := range source {
			spanOptions = append(spanOptions, option)
		}
	}
	return spanOptions
}

func extractB3HeadersToContext(parentCtx context.Context, request *fasthttp.Request) context.Context {
	requestHeadersMapCarrier := propagation.MapCarrier{}
	request.Header.VisitAll(func(key, value []byte) {
		requestHeadersMapCarrier.Set(strings.ToLower(string(key)), string(value))
	})
	contextWithSpanContext := b3Propagator.Extract(parentCtx, requestHeadersMapCarrier)
	if sc := trace.SpanContextFromContext(contextWithSpanContext); sc.IsValid() {
		return contextWithSpanContext
	}
	return parentCtx
}
