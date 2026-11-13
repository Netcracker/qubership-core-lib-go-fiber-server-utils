package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartServerOnAddressListenerContract(t *testing.T) {
	fixture := httptest.NewTLSServer(http.NotFoundHandler())
	cert := fixture.TLS.Certificates[0]
	fixture.Close()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), certPEM, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.crt"), certPEM, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600))
	t.Setenv(utils.TlsPathEnv, dir)
	previousTLS := utils.IsTlsEnabled()
	t.Cleanup(func() { utils.SetTlsEnabled(previousTLS) })
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(certPEM))

	for _, tc := range []struct {
		name    string
		network string
		host    string
		tls     bool
	}{
		{name: "HTTP default IPv4", host: "127.0.0.1"},
		{name: "HTTP tcp4", network: "tcp4", host: "127.0.0.1"},
		{name: "HTTP tcp IPv4", network: "tcp", host: "127.0.0.1"},
		{name: "HTTP tcp IPv6", network: "tcp", host: "::1"},
		{name: "TLS default IPv4", host: "127.0.0.1", tls: true},
		{name: "TLS tcp4", network: "tcp4", host: "127.0.0.1", tls: true},
		{name: "TLS tcp IPv4", network: "tcp", host: "127.0.0.1", tls: true},
		{name: "TLS tcp IPv6", network: "tcp", host: "::1", tls: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.host == "::1" {
				probe, err := net.Listen("tcp6", "[::1]:0")
				if err != nil {
					t.Skipf("IPv6 loopback is unavailable: %v", err)
				}
				require.NoError(t, probe.Close())
			}
			utils.SetTlsEnabled(tc.tls)
			app := fiber.New()
			entered := make(chan struct{})
			release := make(chan struct{})
			releaseRequest := sync.OnceFunc(func() { close(release) })
			app.Get("/ready", func(c fiber.Ctx) error {
				close(entered)
				<-release
				return c.SendString("ready")
			})
			ready := make(chan fiber.ListenData, 1)
			app.Hooks().OnListen(func(data fiber.ListenData) error { ready <- data; return nil })
			stopping := make(chan struct{}, 1)
			app.Hooks().OnPreShutdown(func() error {
				select {
				case stopping <- struct{}{}:
				default:
				}
				return nil
			})
			done := make(chan struct{})
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			t.Cleanup(func() {
				releaseRequest()
				assert.NoError(t, app.ShutdownWithTimeout(3*time.Second))
			})
			go func() {
				defer close(done)
				address := net.JoinHostPort(tc.host, "0")
				if tc.network == "" {
					StartServerOnAddress(app, ":0")
				} else {
					StartServerOnAddress(app, address, fiber.ListenConfig{
						ListenerNetwork: tc.network, DisableStartupMessage: true, GracefulContext: ctx,
					})
				}
			}()
			var listening fiber.ListenData
			select {
			case listening = <-ready:
			case <-time.After(5 * time.Second):
				t.Fatal("StartServerOnAddress did not bind a listener within 5 s")
			}
			if tc.network == "" {
				assert.Equal(t, "0.0.0.0", listening.Host)
			}
			scheme := "http"
			if tc.tls {
				scheme = "https"
			}
			transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			type result struct {
				response *http.Response
				err      error
			}
			responses := make(chan result, 1)
			go func() {
				resp, err := client.Get(scheme + "://" + net.JoinHostPort(tc.host, listening.Port) + "/ready")
				responses <- result{response: resp, err: err}
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach the handler within 5 s")
			}
			shutdownErr := make(chan error, 1)
			if tc.network == "" {
				go func() { shutdownErr <- app.ShutdownWithTimeout(3 * time.Second) }()
			} else {
				cancel()
			}
			select {
			case <-stopping:
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown did not begin within 5 s while the request was active")
			}
			releaseRequest()
			got := <-responses
			resp, err := got.response, got.err
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "ready", string(body))
			if tc.tls {
				require.NotNil(t, resp.TLS)
				assert.NotEmpty(t, resp.TLS.VerifiedChains)
			}
			if tc.network == "" {
				require.NoError(t, <-shutdownErr)
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("StartServerOnAddress did not return within 5 s after shutdown")
			}
		})
	}
}
