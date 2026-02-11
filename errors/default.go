package errors

import (
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v3"
	errs "github.com/netcracker/qubership-core-lib-go-error-handling/v3/errors"
	"github.com/netcracker/qubership-core-lib-go-error-handling/v3/tmf"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
)

var (
	logger = logging.GetLogger("errors")
)

func DefaultErrorHandler(unknownErrorCode errs.ErrorCode) fiber.ErrorHandler {
	return func(ctx *fiber.Ctx, err error) error {
		if hError, ok := err.(interface {
			Handle(ctx *fiber.Ctx) error
		}); ok {
			// if err is not nil after Handle(), then process err here
			if hErr := hError.Handle(ctx); hErr == nil {
				return nil
			} else {
				err = fmt.Errorf("error's Handle() method failed: %s. Original error: %w", hErr.Error(), err)
			}
		}
		var response *tmf.Response
		status := http.StatusInternalServerError
		if e, ok := err.(*fiber.Error); ok {
			status = e.Code
		}
		switch errT := err.(type) {
		case errs.ErrCodeErr:
			logger.ErrorC(ctx.UserContext(), errs.ToLogFormat(errT))
			response = tmf.NewResponseBuilder(errT).Status(status).Build()
		case error:
			unknownError := errs.NewError(unknownErrorCode, errT.Error(), errT)
			logger.ErrorC(ctx.UserContext(), errs.ToLogFormatWithoutStackTrace(unknownError))
			response = tmf.NewResponseBuilder(unknownError).Status(status).Build()
		}
		return ctx.Status(status).JSON(response)
	}
}
