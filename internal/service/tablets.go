package service

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"rsc.io/qr"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// JoinCodeLifetime is how long a tablet has to use a pairing code.
const JoinCodeLifetime = 15 * time.Minute

// maxJoinCodes caps the codes waiting to be used.
const maxJoinCodes = 20

// TabletNetwork says how tablets on the restaurant's network reach Kishtin.
type TabletNetwork struct {
	// Addresses lists this machine's local addresses, as it has them now.
	Addresses func() []net.IP
	// JoinPort serves the join page over plain HTTP, where a tablet starts
	// before it trusts Kishtin's certificate.
	JoinPort string
	// HTTPSPort serves Kishtin to tablets securely.
	HTTPSPort string
	// Fingerprint is Kishtin's own certificate authority, which tablets
	// install once. Empty when kishtin.toml names a certificate of its own,
	// so tablets need not install anything.
	Fingerprint string
}

// JoinCode lets one tablet or phone pair itself, once, within
// JoinCodeLifetime.
type JoinCode struct {
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
	// Links are the addresses a tablet can open to join, one per network
	// this machine is on, most likely first.
	Links []JoinLink `json:"links"`
	// Fingerprint is Kishtin's certificate, as the tablet will show it.
	Fingerprint string `json:"fingerprint,omitempty"`
}

// JoinLink is one address to join at, with its QR code.
type JoinLink struct {
	Address string `json:"address"`
	URL     string `json:"url"`
	// QR is the code's rows, top first: "1" dark, "0" light, with no quiet
	// zone around it.
	QR []string `json:"qr"`
}

type pendingJoin struct {
	name    string
	by      auth.User
	expires time.Time
}

// joins holds codes waiting to be used. They live in memory: a restart
// cancels them, and a manager makes another.
type joins struct {
	mu    sync.Mutex
	codes map[string]pendingJoin
}

// CreateJoinCode makes a one-time code that pairs a new tablet or phone,
// named name, when it scans the QR code or opens the link.
func (s *Service) CreateJoinCode(_ context.Context, p *auth.Principal, name string) (JoinCode, error) {
	if err := s.allow(p, auth.ActManageKeys); err != nil {
		return JoinCode{}, err
	}
	if s.tablets == nil {
		return JoinCode{}, newErr(KindInvalid, "tablets are turned off in kishtin.toml (server.tablets): turn them on and restart Kishtin")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return JoinCode{}, newErr(KindInvalid, "name the device, such as \"Pass tablet\"")
	}
	if len([]rune(name)) > 60 {
		return JoinCode{}, newErr(KindInvalid, "a device name is at most 60 characters")
	}
	code, err := auth.NewCode()
	if err != nil {
		return JoinCode{}, err
	}
	now := s.clk.Now()
	jc := JoinCode{Code: code, Name: name, ExpiresAt: now.Add(JoinCodeLifetime), Fingerprint: s.tablets.Fingerprint}
	for _, ip := range s.tablets.Addresses() {
		link, err := joinLink(ip, s.tablets, code)
		if err != nil {
			return JoinCode{}, err
		}
		jc.Links = append(jc.Links, link)
	}
	if len(jc.Links) == 0 {
		return JoinCode{}, newErr(KindInvalid, "this computer is not on a local network that tablets can reach: connect it to the restaurant's Wi-Fi or network, then try again")
	}

	s.joins.mu.Lock()
	defer s.joins.mu.Unlock()
	if s.joins.codes == nil {
		s.joins.codes = map[string]pendingJoin{}
	}
	for c, pj := range s.joins.codes {
		if !now.Before(pj.expires) {
			delete(s.joins.codes, c)
		}
	}
	if len(s.joins.codes) >= maxJoinCodes {
		return JoinCode{}, newErr(KindConflict, "%d pairing codes are already waiting: use them, or wait %d minutes for them to end", maxJoinCodes, int(JoinCodeLifetime.Minutes()))
	}
	s.joins.codes[auth.NormalCode(code)] = pendingJoin{name: name, by: p.User, expires: jc.ExpiresAt}
	return jc, nil
}

func joinLink(ip net.IP, tn *TabletNetwork, code string) (JoinLink, error) {
	host := ip.String()
	if ip.To4() == nil {
		host = "[" + host + "]"
	}
	u := url.URL{Scheme: "http", Host: host + ":" + tn.JoinPort, Path: "/join", RawQuery: url.Values{"code": {code}}.Encode()}
	if tn.Fingerprint == "" {
		// Kishtin has a certificate tablets already trust: go straight to
		// the secure address.
		u = url.URL{Scheme: "https", Host: host + ":" + tn.HTTPSPort, Path: "/", Fragment: "join=" + code}
	}
	rows, err := qrRows(u.String())
	if err != nil {
		return JoinLink{}, err
	}
	return JoinLink{Address: ip.String(), URL: u.String(), QR: rows}, nil
}

// qrRows draws text as a QR code, medium error correction.
func qrRows(text string) ([]string, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, fmt.Errorf("draw QR code: %w", err)
	}
	rows := make([]string, c.Size)
	for y := range c.Size {
		var b strings.Builder
		for x := range c.Size {
			if c.Black(x, y) {
				b.WriteByte('1')
			} else {
				b.WriteByte('0')
			}
		}
		rows[y] = b.String()
	}
	return rows, nil
}

// Paired is a device key made for a tablet that used a join code. The
// secret is shown once, to that tablet.
type Paired struct {
	NewKeyResult
	Name string `json:"name"`
}

// Pair uses a join code: it makes a device key for the tablet, named as the
// code said. It needs no API key, since the tablet has none yet; the code is
// the proof, and works once.
func (s *Service) Pair(ctx context.Context, code string) (Paired, error) {
	now := s.clk.Now()
	key := auth.NormalCode(code)
	s.joins.mu.Lock()
	pj, ok := s.joins.codes[key]
	if ok {
		delete(s.joins.codes, key)
	}
	s.joins.mu.Unlock()
	if !ok || !now.Before(pj.expires) {
		return Paired{}, newErr(KindNotFound, "this pairing code has ended or was already used: ask a manager for a new QR code")
	}
	var res Paired
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		k, secret, err := s.newKey(ctx, tx, auth.KeyDevice, pj.name, "", pj.by.ID)
		if err != nil {
			return err
		}
		res = Paired{NewKeyResult: NewKeyResult{Key: k, Secret: secret}, Name: pj.name}
		return s.note(ctx, tx, &auth.Principal{User: pj.by, Device: k.ID, DeviceName: k.Name}, ActivityDevicePaired, k.ID, map[string]any{"name": pj.name})
	})
	if err != nil {
		return Paired{}, err
	}
	s.publish(events.KindDevice, res.Key.ID)
	return res, nil
}
