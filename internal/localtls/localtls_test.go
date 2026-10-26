package localtls

import (
	"crypto/tls"
	"crypto/x509"
	b64 "encoding/base64"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func newCA(t *testing.T) (*CA, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tls")
	ca, err := LoadOrCreateCA(dir, "Kishtin on kitchen-pc", "kitchen-pc", now)
	require.NoError(t, err)
	return ca, dir
}

func TestCAIsKeptAndReloaded(t *testing.T) {
	ca, dir := newCA(t)
	assert.True(t, ca.Cert.IsCA)
	assert.Equal(t, "Kishtin on kitchen-pc", ca.Cert.Subject.CommonName)
	again, err := LoadOrCreateCA(dir, "ignored when it exists", "kitchen-pc", now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, ca.Fingerprint(), again.Fingerprint(), "the same authority, so tablets keep trusting it")
	assert.Len(t, strings.Split(ca.Fingerprint(), ":"), 32)
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, "ca-key.pem"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "the key is readable by its owner only")
	}
}

func verify(ca *CA, cert tls.Certificate, name string) error {
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	_, err := cert.Leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: name, CurrentTime: now})
	return err
}

func TestServerCertificateCoversLocalAddressesOnly(t *testing.T) {
	ca, _ := newCA(t)
	ips := []net.IP{net.ParseIP("192.168.1.20"), net.ParseIP("8.8.8.8"), net.ParseIP("10.0.0.5")}
	cert, err := ca.Issue(Names("kitchen-pc"), ips, now)
	require.NoError(t, err)
	for _, name := range []string{"192.168.1.20", "10.0.0.5", "kitchen-pc.local", "kitchen-pc", "localhost"} {
		assert.NoError(t, verify(ca, cert, name), name)
	}
	assert.Error(t, verify(ca, cert, "8.8.8.8"), "a public address is left out")
	assert.LessOrEqual(t, cert.Leaf.NotAfter.Sub(cert.Leaf.NotBefore), 398*24*time.Hour, "within what Apple accepts")
}

func TestAuthorityCannotVouchForTheInternet(t *testing.T) {
	ca, _ := newCA(t)
	// Even a certificate the key signs for an internet name fails, because
	// the authority is limited to local names.
	cert, err := ca.Issue([]string{"bank.example.com"}, nil, now)
	require.NoError(t, err)
	err = verify(ca, cert, "bank.example.com")
	require.Error(t, err)
	var ce x509.CertificateInvalidError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, x509.CANotAuthorizedForThisName, ce.Reason)
}

func TestRefreshReissuesForNewAddressesAndNearExpiry(t *testing.T) {
	ca, _ := newCA(t)
	clock := now
	s, err := NewServer(ca, "kitchen-pc", []net.IP{net.ParseIP("192.168.1.20")}, func() time.Time { return clock })
	require.NoError(t, err)
	first, _ := s.GetCertificate(nil)

	issued, err := s.Refresh([]net.IP{net.ParseIP("192.168.1.20")})
	require.NoError(t, err)
	assert.False(t, issued, "nothing changed")

	issued, err = s.Refresh([]net.IP{net.ParseIP("192.168.1.31")})
	require.NoError(t, err)
	assert.True(t, issued, "the router gave the machine a new address")
	second, _ := s.GetCertificate(nil)
	assert.NotEqual(t, first.Leaf.SerialNumber, second.Leaf.SerialNumber)
	assert.NoError(t, verify(ca, *second, "192.168.1.31"))

	clock = second.Leaf.NotAfter.Add(-30 * 24 * time.Hour)
	issued, err = s.Refresh([]net.IP{net.ParseIP("192.168.1.31")})
	require.NoError(t, err)
	assert.True(t, issued, "renewed before it ends")
}

func TestHandshake(t *testing.T) {
	ca, _ := newCA(t)
	s, err := NewServer(ca, "kitchen-pc", nil, time.Now)
	require.NoError(t, err)
	// Served the way kishtind serves it: only GetCertificate, no fixed
	// certificate. (httptest's StartTLS would add its own.)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{GetCertificate: s.GetCertificate, MinVersion: tls.VersionTLS12})
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }), ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	url := "https://" + ln.Addr().String()

	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	c := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	res, err := c.Get(url)
	require.NoError(t, err)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	assert.Equal(t, "ok", string(body))

	// A device that has not installed the authority is refused.
	_, err = http.Get(url)
	assert.Error(t, err)
}

func TestMobileConfig(t *testing.T) {
	ca, _ := newCA(t)
	mc := ca.MobileConfig("Kishtin & The Synthetic Kitchen")
	var v any
	require.NoError(t, xml.Unmarshal(mc, &v), "well-formed XML")
	s := string(mc)
	assert.Contains(t, s, "com.apple.security.root")
	assert.Contains(t, s, "Kishtin &amp; The Synthetic Kitchen")
	assert.Contains(t, s, ca.Fingerprint())
	flat := strings.Join(strings.Fields(s), "")
	assert.Contains(t, flat, b64.StdEncoding.EncodeToString(ca.DER()))
	assert.Equal(t, string(mc), string(ca.MobileConfig("Kishtin & The Synthetic Kitchen")), "stable, so a second download replaces the first")
}

func TestPrivate(t *testing.T) {
	for ip, want := range map[string]bool{
		"192.168.1.20": true, "10.1.2.3": true, "172.20.0.1": true, "100.100.1.1": true,
		"127.0.0.1": true, "fd12::1": true, "8.8.8.8": false, "172.32.0.1": false, "2001:db8::1": false,
	} {
		assert.Equal(t, want, Private(net.ParseIP(ip)), ip)
	}
}
