package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartServerOnAddressListenerNetwork(t *testing.T) {
	tests := []struct {
		name          string
		tls           bool
		network       string
		dialHost      string
		wantReachable bool
	}{
		{name: "HTTP default network accepts IPv4", network: "", dialHost: "127.0.0.1", wantReachable: true},
		{name: "HTTP default network refuses IPv6", network: "", dialHost: "::1", wantReachable: false},
		{name: "HTTP tcp4 accepts IPv4", network: "tcp4", dialHost: "127.0.0.1", wantReachable: true},
		{name: "HTTP tcp4 refuses IPv6", network: "tcp4", dialHost: "::1", wantReachable: false},
		{name: "HTTP tcp accepts IPv4", network: "tcp", dialHost: "127.0.0.1", wantReachable: true},
		{name: "HTTP tcp accepts IPv6", network: "tcp", dialHost: "::1", wantReachable: true},
		{name: "TLS default network accepts IPv4", tls: true, network: "", dialHost: "127.0.0.1", wantReachable: true},
		{name: "TLS default network refuses IPv6", tls: true, network: "", dialHost: "::1", wantReachable: false},
		{name: "TLS tcp4 accepts IPv4", tls: true, network: "tcp4", dialHost: "127.0.0.1", wantReachable: true},
		{name: "TLS tcp4 refuses IPv6", tls: true, network: "tcp4", dialHost: "::1", wantReachable: false},
		{name: "TLS tcp accepts IPv4", tls: true, network: "tcp", dialHost: "127.0.0.1", wantReachable: true},
		{name: "TLS tcp accepts IPv6", tls: true, network: "tcp", dialHost: "::1", wantReachable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.dialHost == "::1" && !ipv6LoopbackAvailable() {
				t.Skip("IPv6 loopback is not available on this machine")
			}
			if tt.tls {
				enableTLS(t)
			}
			port := freePort(t)
			app := fiber.New(fiber.Config{Network: tt.network, DisableStartupMessage: true})
			app.Get("/ok", func(c *fiber.Ctx) error {
				return c.SendString("ok")
			})
			go StartServerOnAddress(app, ":"+strconv.Itoa(port))
			t.Cleanup(func() { _ = app.Shutdown() })
			client := testClient()
			scheme := "http"
			if tt.tls {
				scheme = "https"
			}
			ipv4URL := fmt.Sprintf("%s://127.0.0.1:%d/ok", scheme, port)
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				_, err := get(client, ipv4URL)
				assert.NoError(c, err, "GET %s", ipv4URL)
			}, 10*time.Second, 50*time.Millisecond)

			target := fmt.Sprintf("%s://%s/ok", scheme, net.JoinHostPort(tt.dialHost, strconv.Itoa(port)))
			body, err := get(client, target)

			assert.Equal(t, tt.wantReachable, err == nil, "GET %s: body %q, error %v", target, body, err)
		})
	}
}

func get(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return string(body), fmt.Errorf("status %d", resp.StatusCode)
	}
	return string(body), nil
}

func testClient() *http.Client {
	return &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func ipv6LoopbackAvailable() bool {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func enableTLS(t *testing.T) {
	t.Helper()
	certDir := writeSelfSignedCertificate(t)
	t.Setenv(utils.TlsPathEnv, certDir)
	utils.SetTlsEnabled(true)
	t.Cleanup(func() { utils.SetTlsEnabled(false) })
}

func writeSelfSignedCertificate(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, utils.TlsCrt), certPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, utils.TlsKey), keyPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, utils.CaCrt), certPEM, 0o600))
	return dir
}
