// Package localtls gives Kishtin HTTPS on the restaurant's own network
// without the internet. Kishtin makes its own certificate authority once,
// each tablet trusts it once, and Kishtin issues itself a server certificate
// for this machine's local addresses. Browsers only allow the microphone and
// screen wake lock on a secure page, so tablets need this for voice and to
// stay awake.
//
// The authority can only vouch for local names and private addresses (name
// constraints), so a tablet that trusts it trusts nothing on the internet
// because of it, even if its key were copied.
package localtls

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	b64 "encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Lifetimes. The server certificate stays under the 398 days Apple accepts
// and is renewed well before it ends.
const (
	caLifetime   = 10 * 365 * 24 * time.Hour
	leafLifetime = 397 * 24 * time.Hour
	renewBefore  = 60 * 24 * time.Hour
)

// privateRanges are the addresses the authority may vouch for: loopback,
// private networks, link-local and carrier-grade NAT (used by VPNs such as
// Tailscale).
var privateRanges = mustNets(
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"169.254.0.0/16", "100.64.0.0/10",
	"::1/128", "fc00::/7", "fe80::/10",
)

func mustNets(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

// Private reports whether ip is an address the authority may vouch for.
func Private(ip net.IP) bool {
	for _, n := range privateRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// CA is Kishtin's own certificate authority.
type CA struct {
	Cert *x509.Certificate
	key  crypto.Signer
}

// DER is the authority's certificate, as tablets install it.
func (ca *CA) DER() []byte { return ca.Cert.Raw }

// Fingerprint is the SHA-256 of the authority's certificate, in pairs of
// hex digits, so a person can check the one a tablet installed.
func (ca *CA) Fingerprint() string {
	sum := sha256.Sum256(ca.Cert.Raw)
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	var parts []string
	for i := 0; i < len(h); i += 2 {
		parts = append(parts, h[i:i+2])
	}
	return strings.Join(parts, ":")
}

// LoadOrCreateCA reads the authority from dir, or makes one there. name
// identifies it on tablets, for example "Kishtin on kitchen-pc". The key
// file is readable by its owner only.
func LoadOrCreateCA(dir, name, hostname string, now time.Time) (*CA, error) {
	certPath, keyPath := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca-key.pem")
	ca, err := loadCA(certPath, keyPath)
	if err == nil {
		return ca, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("make certificate authority key: %w", err)
	}
	dns := []string{"local", "localhost"}
	if h := strings.ToLower(strings.TrimSuffix(hostname, ".local")); h != "" && !strings.Contains(h, ".") && h != "localhost" {
		dns = append(dns, h)
	}
	tmpl := &x509.Certificate{
		SerialNumber:                serial(),
		Subject:                     pkix.Name{CommonName: name, Organization: []string{"Kishtin"}},
		NotBefore:                   now.Add(-time.Hour),
		NotAfter:                    now.Add(caLifetime),
		IsCA:                        true,
		BasicConstraintsValid:       true,
		MaxPathLenZero:              true,
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         dns,
		PermittedIPRanges:           privateRanges,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return nil, fmt.Errorf("make certificate authority: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return nil, fmt.Errorf("save certificate authority key: %w", err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return nil, fmt.Errorf("save certificate authority: %w", err)
	}
	return loadCA(certPath, keyPath)
}

func loadCA(certPath, keyPath string) (*CA, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	cb, _ := pem.Decode(certPEM)
	kb, _ := pem.Decode(keyPEM)
	if cb == nil || kb == nil {
		return nil, fmt.Errorf("read %s: not a PEM certificate and key", filepath.Dir(certPath))
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", certPath, err)
	}
	k, err := x509.ParsePKCS8PrivateKey(kb.Bytes)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", keyPath, err)
	}
	signer, ok := k.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("read %s: unsupported key", keyPath)
	}
	return &CA{Cert: cert, key: signer}, nil
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		panic(err)
	}
	return n
}

// Issue makes a server certificate for the given names and addresses.
// Addresses the authority may not vouch for are left out.
func (ca *CA) Issue(names []string, ips []net.IP, now time.Time) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	var keep []net.IP
	for _, ip := range ips {
		if Private(ip) {
			keep = append(keep, ip)
		}
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: "Kishtin"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(leafLifetime),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     names,
		IPAddresses:  keep,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, key.Public(), ca.key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("make server certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.Cert.Raw}, PrivateKey: key, Leaf: leaf}, nil
}

// Names are the DNS names a server certificate covers for this machine.
func Names(hostname string) []string {
	names := []string{"localhost"}
	h := strings.ToLower(strings.TrimSuffix(hostname, ".local"))
	if h != "" && !strings.Contains(h, ".") && h != "localhost" {
		names = append(names, h, h+".local")
	}
	return names
}

// Server keeps a current server certificate for this machine's addresses.
type Server struct {
	ca       *CA
	hostname string
	now      func() time.Time

	mu   sync.RWMutex
	cert *tls.Certificate
}

// NewServer issues a first certificate for the given addresses.
func NewServer(ca *CA, hostname string, ips []net.IP, now func() time.Time) (*Server, error) {
	s := &Server{ca: ca, hostname: hostname, now: now}
	if _, err := s.Refresh(ips); err != nil {
		return nil, err
	}
	return s, nil
}

// Refresh issues a new certificate when an address is not covered or the
// current one is near its end. It reports whether it issued one.
func (s *Server) Refresh(ips []net.IP) (bool, error) {
	s.mu.RLock()
	cur := s.cert
	s.mu.RUnlock()
	if cur != nil && covers(cur.Leaf, ips) && s.now().Add(renewBefore).Before(cur.Leaf.NotAfter) {
		return false, nil
	}
	all := append([]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, ips...)
	cert, err := s.ca.Issue(Names(s.hostname), all, s.now())
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	s.cert = &cert
	s.mu.Unlock()
	return true, nil
}

func covers(leaf *x509.Certificate, ips []net.IP) bool {
	for _, ip := range ips {
		if !Private(ip) {
			continue
		}
		if !slices.ContainsFunc(leaf.IPAddresses, func(have net.IP) bool { return have.Equal(ip) }) {
			return false
		}
	}
	return true
}

// GetCertificate serves the current certificate, for tls.Config.
func (s *Server) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cert, nil
}

// Addresses lists this machine's addresses on local networks, IPv4 first:
// the ones tablets can reach it at.
func Addresses() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var v4, v6 []net.IP
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || !Private(ipn.IP) || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			if ip4 := ipn.IP.To4(); ip4 != nil {
				v4 = append(v4, ip4)
			} else {
				v6 = append(v6, ipn.IP)
			}
		}
	}
	return append(v4, v6...)
}

// MobileConfig is an Apple configuration profile that installs the
// authority on an iPhone or iPad. It is not signed, so iOS shows it as not
// verified; the fingerprint on the profile matches the one on Kishtin's
// screen.
func (ca *CA) MobileConfig(label string) []byte {
	fp := strings.ReplaceAll(ca.Fingerprint(), ":", "")
	id := "ie.kishtin.ca." + strings.ToLower(fp[:16])
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadCertificateFileName</key>
			<string>kishtin-ca.cer</string>
			<key>PayloadContent</key>
			<data>`)
	b.WriteString(wrapped(ca.DER()))
	fmt.Fprintf(&b, `</data>
			<key>PayloadDescription</key>
			<string>Lets this device open Kishtin securely on the restaurant network.</string>
			<key>PayloadDisplayName</key>
			<string>%s</string>
			<key>PayloadIdentifier</key>
			<string>%s.cert</string>
			<key>PayloadType</key>
			<string>com.apple.security.root</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
		</dict>
	</array>
	<key>PayloadDescription</key>
	<string>Kishtin's own certificate for this restaurant. It only covers local network addresses. Fingerprint (SHA-256): %s</string>
	<key>PayloadDisplayName</key>
	<string>%s</string>
	<key>PayloadIdentifier</key>
	<string>%s</string>
	<key>PayloadOrganization</key>
	<string>Kishtin</string>
	<key>PayloadRemovalDisallowed</key>
	<false/>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>%s</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>
`, xmlText(label), id, uuidFrom(fp, "cert"), ca.Fingerprint(), xmlText(label), id, uuidFrom(fp, "profile"))
	return b.Bytes()
}

// wrapped is b in base64, in lines of 64 for the profile.
func wrapped(b []byte) string {
	var sb strings.Builder
	enc := []byte(b64.StdEncoding.EncodeToString(b))
	for i := 0; i < len(enc); i += 64 {
		end := min(i+64, len(enc))
		sb.WriteString("\n\t\t\t")
		sb.Write(enc[i:end])
	}
	sb.WriteString("\n\t\t\t")
	return sb.String()
}

func xmlText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// uuidFrom makes a stable UUID-shaped identifier from the fingerprint, so
// re-downloading the profile replaces the installed one instead of adding
// another.
func uuidFrom(fp, what string) string {
	sum := sha256.Sum256([]byte(fp + "/" + what))
	h := hex.EncodeToString(sum[:16])
	return strings.ToUpper(h[0:8] + "-" + h[8:12] + "-4" + h[13:16] + "-a" + h[17:20] + "-" + h[20:32])
}
