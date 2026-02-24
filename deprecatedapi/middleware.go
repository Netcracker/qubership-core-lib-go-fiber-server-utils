package deprecatedapi

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/vibrantbyte/go-antpath/antpath"
)

var (
	logger                         = logging.GetLogger("deprecatedapi")
	PathPattern                    = regexp.MustCompile(`\s*(\S+)\s*(\[([^\[\]]+)]\s*)?`)
	FiberParamsAndWildcardsPattern = regexp.MustCompile(`(:[^/\.-]*[?]?)|([+*])`)
)

type DisabledUrlPatterns struct {
	urlsMap        map[string][]string
	antPathPattern *antpath.AntPathMatcher
}

func DisableDeprecatedApi(app *fiber.App) error {
	disabled, err := strconv.ParseBool(configloader.GetOrDefaultString("deprecated.api.disabled", "false"))
	if err != nil {
		return err
	}
	if disabled {
		if !configloader.GetKoanf().Exists("deprecated.api.patterns") {
			return fmt.Errorf("property 'deprecated.api.patterns' must be provided when property 'deprecated.api.disabled'=true")
		}
		patterns := configloader.GetKoanf().Get("deprecated.api.patterns").([]interface{})
		urlsMap := ConvertPatterns(patterns)
		urlPatterns := &DisabledUrlPatterns{
			urlsMap:        urlsMap,
			antPathPattern: antpath.New(),
		}
		app.Use(urlPatterns.disableDeprecatedApiHandler)
		app.Hooks().OnListen(func(data fiber.ListenData) error {
			disabledRoutesLines := getDisabledEndpoints(app, urlPatterns)
			logger.Warnf("Disabling the following deprecated paths: \n%s", strings.Join(disabledRoutesLines, "\n"))
			return nil
		})
	}
	return nil
}

func ConvertPatterns(patterns []interface{}) map[string][]string {
	urlsMap := make(map[string][]string)
	for _, antPath := range patterns {
		path := antPath.(string)
		matched := PathPattern.FindStringSubmatch(path)
		if matched == nil || len(matched) != 4 {
			panic(fmt.Sprintf("invalid path: '%s' valid pattern: '%s'", path, PathPattern))
		}
		antPattern := matched[1]
		methodsStr := strings.TrimSpace(matched[3])
		var methods []string
		if len(strings.TrimSpace(methodsStr)) != 0 {
			methods = strings.Split(regexp.MustCompile(`[\s,]+`).ReplaceAllString(strings.ToUpper(methodsStr), ","), ",")
		} else {
			methods = []string{"*"}
		}
		urlsMap[antPattern] = methods
	}
	return urlsMap
}

func getDisabledEndpoints(app *fiber.App, urlPatterns *DisabledUrlPatterns) []string {
	disabledRoutes := make(map[string][]string)
	for _, group := range app.Stack() {
		for _, route := range group {
			// for matching check below, need to convert fiber regex patterns to corresponding real paths
			// replace all '(:[^/\.-]*\[?]?)' and '([+*])' with 'param' string
			simplePath := FiberParamsAndWildcardsPattern.ReplaceAllString(route.Path, "param")
			_, methods := urlPatterns.getDisabledPathAndMethods(simplePath, route.Method)
			if methods != nil {
				pathMethods, ok := disabledRoutes[route.Path]
				if ok {
					pathMethods = append(pathMethods, route.Method)
				} else {
					pathMethods = []string{route.Method}
				}
				disabledRoutes[route.Path] = pathMethods
			}
		}
	}
	keys := make([]string, 0, len(disabledRoutes))
	for k := range disabledRoutes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var disabledRoutesLines []string
	for _, path := range keys {
		disabledRoutesLines = append(disabledRoutesLines, fmt.Sprintf("%s [%s]", path, strings.Join(disabledRoutes[path], ",")))
	}
	return disabledRoutesLines
}

func (p *DisabledUrlPatterns) disableDeprecatedApiHandler(ctx *fiber.Ctx) error {
	reqUri := string(ctx.Request().RequestURI())
	reqMethod := ctx.Method()
	antPath, methods := p.getDisabledPathAndMethods(reqUri, reqMethod)
	if methods != nil {
		status := fiber.StatusNotFound
		response := CreateErrorResponse(reqMethod, reqUri, methods, antPath, status)
		ctx.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return ctx.Status(status).JSON(response)
	}
	return ctx.Next()
}

func (p *DisabledUrlPatterns) getDisabledPathAndMethods(uri string, method string) (string, []string) {
	for antPath, methods := range p.urlsMap {
		if p.antPathPattern.Match(antPath, uri) {
			for _, m := range methods {
				if m == method || m == "*" {
					// deprecated api case
					return antPath, methods
				}
			}
		}
	}
	return "", nil
}
