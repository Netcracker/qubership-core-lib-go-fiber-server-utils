package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v3"
	errs "github.com/netcracker/qubership-core-lib-go-error-handling/v3/errors"
	"github.com/netcracker/qubership-core-lib-go-error-handling/v3/tmf"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestDefaultErrorHandler(t *testing.T) {
	assert := require.New(t)
	unknownErrorCode := errs.ErrorCode{Code: "test code", Title: "test title"}
	app := fiber.New()
	c := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(c)
	testErr := fmt.Errorf("test error")
	err := DefaultErrorHandler(unknownErrorCode)(c, testErr)
	assert.Nil(err)
	response := c.Response()
	assert.NotNil(response)
	body := response.Body()
	tmfResponse := tmf.Response{}
	err = json.Unmarshal(body, &tmfResponse)
	assert.Nil(err)
	assert.Equal(unknownErrorCode.Code, tmfResponse.Code)
	assert.Equal(unknownErrorCode.Title, tmfResponse.Reason)
	assert.Equal("test error", tmfResponse.Message)
	assert.Equal("500", *tmfResponse.Status)
	assert.Equal(tmf.TypeV1_0, tmfResponse.Type)
	assert.Equal(fiber.MIMEApplicationJSON, string(response.Header.ContentType()))
}

type CustomErr struct {
	*errs.ErrCodeError
	CustomField string
}

func NewCustomErr(detail string) *CustomErr {
	return errs.New(CustomErr{CustomField: detail}, errs.ErrorCode{Code: "custom test error", Title: "custom test title"}, detail)
}

func (e *CustomErr) Handle(ctx *fiber.Ctx) error {
	status := http.StatusBadRequest
	response := tmf.NewResponseBuilder(e).
		Meta(map[string]interface{}{"custom": e.CustomField}).
		Status(status).
		Build()
	return ctx.Status(status).JSON(response)
}

func TestDefaultErrorHandlerCustomErr(t *testing.T) {
	assert := require.New(t)
	unknownErrorCode := errs.ErrorCode{Code: "test code", Title: "test title"}
	app := fiber.New()
	c := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(c)
	testErr := NewCustomErr("custom test details")
	err := DefaultErrorHandler(unknownErrorCode)(c, testErr)
	assert.Nil(err)
	response := c.Response()
	assert.NotNil(response)
	body := response.Body()
	tmfResponse := tmf.Response{}
	err = json.Unmarshal(body, &tmfResponse)
	assert.Nil(err)
	assert.Equal(testErr.GetErrorCode().Code, tmfResponse.Code)
	assert.Equal(testErr.GetErrorCode().Title, tmfResponse.Reason)
	assert.Equal(testErr.GetDetail(), tmfResponse.Message)
	assert.Equal(strconv.Itoa(http.StatusBadRequest), *tmfResponse.Status)
	assert.Equal("custom test details", (*tmfResponse.Meta)["custom"].(string))
	assert.Equal(tmf.TypeV1_0, tmfResponse.Type)
	assert.Equal(fiber.MIMEApplicationJSON, string(response.Header.ContentType()))
}

type customErrWithBadHandleFunc struct {
	*errs.ErrCodeError
}

func newCustomErrWithBadHandleFunc(detail string) *customErrWithBadHandleFunc {
	customErr := customErrWithBadHandleFunc{
		ErrCodeError: errs.NewError(errs.ErrorCode{
			Code:  "custom test error",
			Title: "custom test title",
		}, detail, nil),
	}
	return &customErr
}

func (e *customErrWithBadHandleFunc) Handle(ctx *fiber.Ctx) error {
	return errors.New("test error from Handle()")
}

func TestDefaultErrorHandlerCustomErrWithBadHandleFunc(t *testing.T) {
	assert := require.New(t)
	unknownErrorCode := errs.ErrorCode{Code: "test code", Title: "test title"}
	app := fiber.New()
	c := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(c)
	testErr := newCustomErrWithBadHandleFunc("custom bad Handle() test details")
	err := DefaultErrorHandler(unknownErrorCode)(c, testErr)
	assert.Nil(err)
	response := c.Response()
	assert.NotNil(response)
	body := response.Body()
	tmfResponse := tmf.Response{}
	err = json.Unmarshal(body, &tmfResponse)
	assert.Nil(err)
	assert.Equal(unknownErrorCode.Code, tmfResponse.Code)
	assert.Equal(unknownErrorCode.Title, tmfResponse.Reason)
	expectedMessage := fmt.Sprintf("error's Handle() method failed: test error from Handle(). "+
		"Original error: ErrCodeError [custom test error][%s] custom bad Handle() test details", testErr.GetId())
	assert.Equal(expectedMessage, tmfResponse.Message)
	assert.Equal(strconv.Itoa(http.StatusInternalServerError), *tmfResponse.Status)
	assert.Equal(tmf.TypeV1_0, tmfResponse.Type)
	assert.Equal(fiber.MIMEApplicationJSON, string(response.Header.ContentType()))
}
