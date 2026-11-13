package fiberserver

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *TestSuite) TestRequestContextPropagatesTenantAndRequestID() {
	t := suite.T()
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	app, err := New().ProcessWithContext(parent)
	require.NoError(t, err)
	var outgoing *http.Request
	app.Get("/context", func(c fiber.Ctx) error {
		outgoing, err = http.NewRequestWithContext(c.Context(), http.MethodGet, "http://upstream/test", nil)
		if err != nil {
			return err
		}
		if err := ctxhelper.AddSerializableContextData(c.Context(), outgoing.Header.Set); err != nil {
			return err
		}
		return c.SendString("propagated")
	})
	request := httptest.NewRequest(http.MethodGet, "/context", nil)
	request.Header.Set("Tenant", "tenant-a")
	request.Header.Set("X-Request-Id", "request-a")
	request.Header.Set("Accept-Language", "en")
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.NotNil(t, outgoing)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "tenant-a", outgoing.Header.Get("Tenant"))
	assert.Equal(t, "request-a", outgoing.Header.Get("X-Request-Id"))
	assert.Equal(t, "en", outgoing.Header.Get("Accept-Language"))
	assert.Equal(t, "request-a", response.Header.Get("X-Request-Id"))

	cancel()
	assert.ErrorIs(t, outgoing.Context().Err(), context.Canceled)
}

func (suite *TestSuite) TestReadBufferSizeBoundsRequestHeaders() {
	for _, tc := range []struct {
		name   string
		size   int
		status int
	}{
		{name: "below limit", size: 900, status: http.StatusOK},
		{name: "above limit", size: 1100, status: http.StatusRequestHeaderFieldsTooLarge},
	} {
		suite.T().Run(tc.name, func(t *testing.T) {
			app, err := New(fiber.Config{ReadBufferSize: 1024}).Process()
			require.NoError(t, err)
			app.Get("/headers", func(c fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			require.NoError(t, err)
			listenErr := make(chan error, 1)
			go func() { listenErr <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true}) }()
			t.Cleanup(func() {
				assert.NoError(t, app.ShutdownWithTimeout(3*time.Second))
				select {
				case err := <-listenErr:
					assert.NoError(t, err)
				case <-time.After(3 * time.Second):
					t.Error("header test listener did not stop within 3 s")
				}
			})
			request, err := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+"/headers", nil)
			require.NoError(t, err)
			request.Header.Set("X-Large", strings.Repeat("x", tc.size))
			transport := &http.Transport{}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			response, err := client.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			assert.Equal(t, tc.status, response.StatusCode)
		})
	}
}
