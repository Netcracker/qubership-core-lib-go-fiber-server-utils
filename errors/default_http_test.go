package errors

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	errs "github.com/netcracker/qubership-core-lib-go-error-handling/v3/errors"
	"github.com/netcracker/qubership-core-lib-go-error-handling/v3/tmf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tmfFields struct {
	Code    string
	Message string
	Status  string
}

func TestDefaultErrorHandlerResponseOverHTTP(t *testing.T) {
	unknownErrorCode := errs.ErrorCode{Code: "TEST-0001", Title: "unexpected test error"}
	knownErrorCode := errs.ErrorCode{Code: "TEST-0002", Title: "known test error"}

	tests := []struct {
		name            string
		handlerErr      error
		wantStatus      int
		wantContentType string
		wantBody        tmfFields
	}{
		{
			name:            "error with an error code is answered 500 with its code",
			handlerErr:      errs.NewError(knownErrorCode, "known failure", nil),
			wantStatus:      http.StatusInternalServerError,
			wantContentType: "application/json",
			wantBody:        tmfFields{Code: "TEST-0002", Message: "known failure", Status: "500"},
		},
		{
			name:            "fiber error keeps its status under the unknown error code",
			handlerErr:      fiber.NewError(http.StatusConflict, "conflict"),
			wantStatus:      http.StatusConflict,
			wantContentType: "application/json",
			wantBody:        tmfFields{Code: "TEST-0001", Message: "conflict", Status: "409"},
		},
		{
			name:            "plain error is answered 500 under the unknown error code",
			handlerErr:      errors.New("plain failure"),
			wantStatus:      http.StatusInternalServerError,
			wantContentType: "application/json",
			wantBody:        tmfFields{Code: "TEST-0001", Message: "plain failure", Status: "500"},
		},
		{
			name:            "error with Handle writes its own response",
			handlerErr:      NewCustomErr("handled failure"),
			wantStatus:      http.StatusBadRequest,
			wantContentType: "application/json",
			wantBody:        tmfFields{Code: "custom test error", Message: "handled failure", Status: "400"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New(fiber.Config{ErrorHandler: DefaultErrorHandler(unknownErrorCode)})
			app.Get("/fail", func(c *fiber.Ctx) error {
				return tt.handlerErr
			})

			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/fail", nil))
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			assert.Equal(t, tt.wantContentType, resp.Header.Get("Content-Type"))
			var response tmf.Response
			require.NoError(t, json.Unmarshal(body, &response), "response body %q", body)
			require.NotNil(t, response.Status, "response body %q", body)
			assert.Equal(t, tt.wantBody, tmfFields{Code: response.Code, Message: response.Message, Status: *response.Status})
		})
	}
}
