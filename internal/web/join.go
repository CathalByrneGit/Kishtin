package web

import (
	"crypto/rand"
	"encoding/base64"
	"html/template"
	"net"
	"net/http"
	"strings"
)

// Tablets is what the join pages need: Kishtin's own certificate, which a
// tablet installs once, and where tablets reach Kishtin over HTTPS.
type Tablets struct {
	// CA is the certificate authority, DER encoded; MobileConfig installs it
	// on an iPhone or iPad.
	CA           []byte
	MobileConfig []byte
	Fingerprint  string
	HTTPSPort    string
}

// SetTablets turns on the join pages a tablet opens from a pairing QR code.
func (s *Server) SetTablets(t *Tablets) { s.tablets = t }

func (s *Server) joinRoutes() {
	s.mux.HandleFunc("GET /join", s.serveJoin)
	s.mux.HandleFunc("GET /join/kishtin-ca.crt", func(w http.ResponseWriter, r *http.Request) {
		s.serveCA(w, r, "application/x-x509-ca-cert", "kishtin-ca.crt", func(t *Tablets) []byte { return t.CA })
	})
	s.mux.HandleFunc("GET /join/kishtin.mobileconfig", func(w http.ResponseWriter, r *http.Request) {
		s.serveCA(w, r, "application/x-apple-aspen-config", "kishtin.mobileconfig", func(t *Tablets) []byte { return t.MobileConfig })
	})
	// The join page, served over plain HTTP, asks for this over HTTPS to
	// learn whether the tablet trusts Kishtin's certificate yet.
	s.mux.HandleFunc("GET /join/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusNoContent)
	})
}

func (s *Server) serveCA(w http.ResponseWriter, _ *http.Request, typ, name string, body func(*Tablets) []byte) {
	if s.tablets == nil {
		http.Error(w, "Tablets are turned off in kishtin.toml (server.tablets).", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write(body(s.tablets))
}

// ForNetwork is the handler for plain HTTP on every network address. This
// computer gets all of Kishtin, as on loopback. Other devices get only the
// join pages, where a tablet starts before it trusts Kishtin's
// certificate; everything else sends them to HTTPS.
func (s *Server) ForNetwork() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestFromThisMachine(r) || r.URL.Path == "/join" || strings.HasPrefix(r.URL.Path, "/join/") {
			s.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if s.tablets == nil {
			http.Error(w, "Kishtin only answers this computer.", http.StatusForbidden)
			return
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			host = h
		}
		if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
			host = "[" + host + "]"
		}
		to := "https://" + host + ":" + s.tablets.HTTPSPort + r.URL.RequestURI()
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Other devices use Kishtin over HTTPS: "+to, http.StatusForbidden)
			return
		}
		http.Redirect(w, r, to, http.StatusFound)
	})
}

// requestFromThisMachine reports whether a plain HTTP request came from
// this computer. A request passed on by a proxy counts as from elsewhere.
func requestFromThisMachine(r *http.Request) bool {
	for _, h := range []string{"Forwarded", "X-Forwarded-For", "X-Real-Ip"} {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type joinPage struct {
	Nonce       string
	Code        string
	HTTPSPort   string
	Fingerprint string
	Off         bool
}

func (s *Server) serveJoin(w http.ResponseWriter, r *http.Request) {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	p := joinPage{Nonce: base64.StdEncoding.EncodeToString(nonce), Code: r.URL.Query().Get("code"), Off: s.tablets == nil}
	if s.tablets != nil {
		p.HTTPSPort, p.Fingerprint = s.tablets.HTTPSPort, s.tablets.Fingerprint
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'nonce-"+p.Nonce+"'; connect-src https:; base-uri 'none'; form-action 'none'")
	_ = joinTmpl.Execute(w, p)
}

var joinTmpl = template.Must(template.New("join").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Connect to Kishtin</title>
<style>
:root{--bg:#f6f7f9;--surface:#fff;--text:#14171c;--muted:#4a5260;--line:#c9ced6;--accent:#0b4fbf;--ok-bg:#e3f4e8;--ok:#15652e;--warn-bg:#fdf3dc;--warn:#7a4b00}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);font:19px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif}
main{max-width:640px;margin:0 auto;padding:24px 16px 48px}
h1{font-size:30px;margin:0 0 4px}
.muted{color:var(--muted)}
.card{background:var(--surface);border:1px solid var(--line);border-radius:10px;padding:18px;margin:18px 0}
.status{border-radius:10px;padding:14px 16px;margin:18px 0;font-weight:600}
.status.wait{background:var(--warn-bg);color:var(--warn)}
.status.ok{background:var(--ok-bg);color:var(--ok)}
a.button,button{display:flex;align-items:center;justify-content:center;min-height:56px;width:100%;margin:10px 0;padding:0 16px;border-radius:8px;border:1px solid var(--accent);font:600 19px system-ui,sans-serif;text-decoration:none;color:var(--accent);background:var(--surface);cursor:pointer}
a.button.primary,button.primary{background:var(--accent);color:#fff}
details{border-top:1px solid var(--line);padding:10px 0}
summary{font-weight:600;cursor:pointer;min-height:44px;display:flex;align-items:center;justify-content:space-between;list-style:none}
summary::-webkit-details-marker{display:none}
summary::after{content:"Show steps";font-weight:400;color:var(--accent)}
details[open] summary::after{content:"Hide"}
ol{padding-left:22px;margin:8px 0}
li{margin:6px 0}
input{width:100%;min-height:56px;font:600 24px ui-monospace,monospace;letter-spacing:2px;text-transform:uppercase;padding:0 12px;border:1px solid var(--line);border-radius:8px}
code{font:16px ui-monospace,monospace;word-break:break-all}
[hidden]{display:none!important}
</style></head><body><main>
<h1>Connect to Kishtin</h1>
<p class="muted">Kishtin, from <i>cistin</i>, Irish for kitchen.</p>
{{if .Off}}
<div class="status wait">Tablets are turned off on this Kishtin. A manager can turn them on in kishtin.toml (server.tablets).</div>
{{else}}
<div id="waiting" class="status wait">Step 1 of 2: let this device trust Kishtin. Then come back to this page.</div>
<div id="trusted" class="status ok" hidden>This device trusts Kishtin.</div>

<div class="card" id="code-card">
<label for="code"><strong>Pairing code</strong></label>
<p class="muted">From the QR code a manager showed you. It works once, for 15 minutes.</p>
<input id="code" value="{{.Code}}" autocomplete="off" autocapitalize="characters" spellcheck="false" placeholder="ABCD-EFGH-JKMN">
<a id="continue" class="button primary" hidden>Continue to Kishtin</a>
</div>

<div class="card" id="steps">
<p><strong>Why:</strong> Kishtin runs on a computer in the restaurant, not on the internet. It has its own security certificate, which each tablet installs once. After that, the connection is private, and voice and keeping the screen awake work. It works without the internet.</p>
<p class="muted">The certificate only covers this restaurant's own network. Its fingerprint (SHA-256) is <code>{{.Fingerprint}}</code>, the same as on Kishtin's screen.</p>

<details id="ios"><summary>iPhone or iPad</summary><ol>
<li><a class="button primary" href="/join/kishtin.mobileconfig">Download Kishtin's certificate</a> Tap <b>Allow</b>, then <b>Close</b>.</li>
<li>Open <b>Settings</b>. Tap <b>Profile Downloaded</b> near the top, then <b>Install</b>. It says "Not Verified": that is expected for Kishtin's own certificate. Enter the passcode and tap <b>Install</b> again.</li>
<li>In Settings, go to <b>General › About › Certificate Trust Settings</b> and turn on <b>Kishtin</b>. Tap <b>Continue</b>.</li>
<li>Come back to this page.</li>
</ol></details>

<details id="android"><summary>Android phone or tablet</summary><ol>
<li><a class="button primary" href="/join/kishtin-ca.crt">Download Kishtin's certificate</a></li>
<li>Open <b>Settings</b> and search for <b>CA certificate</b>. (It is usually under Security › More security settings › Encryption and credentials › Install a certificate.)</li>
<li>Tap <b>CA certificate</b>, then <b>Install anyway</b>, and choose <b>kishtin-ca.crt</b> from Downloads.</li>
<li>Come back to this page.</li>
</ol></details>

<details id="windows"><summary>Windows computer</summary><ol>
<li><a class="button primary" href="/join/kishtin-ca.crt">Download Kishtin's certificate</a></li>
<li>Open the downloaded file and click <b>Install Certificate</b>. Choose <b>Current User</b>, then <b>Place all certificates in the following store</b>, click <b>Browse</b> and choose <b>Trusted Root Certification Authorities</b>. Click <b>Finish</b>, then <b>Yes</b>.</li>
<li>Close the browser completely, open it again, and come back to this page.</li>
</ol></details>

<details id="mac"><summary>Mac</summary><ol>
<li><a class="button primary" href="/join/kishtin-ca.crt">Download Kishtin's certificate</a></li>
<li>Open the downloaded file. Keychain Access adds it. Double-click <b>Kishtin</b> in the list, open <b>Trust</b>, and set <b>When using this certificate</b> to <b>Always Trust</b>. Close the window and enter your password.</li>
<li>Come back to this page.</li>
</ol></details>

<details id="chromeos"><summary>Chromebook</summary><ol>
<li><a class="button primary" href="/join/kishtin-ca.crt">Download Kishtin's certificate</a></li>
<li>Open <b>Settings › Privacy and security › Security › Manage certificates › Authorities</b>, click <b>Import</b>, and choose <b>kishtin-ca.crt</b>. Tick <b>Trust this certificate for identifying websites</b>.</li>
<li>Come back to this page.</li>
</ol></details>

<p class="muted">Using Firefox? It keeps its own list: Settings › Privacy &amp; Security › Certificates › View Certificates › Authorities › Import.</p>
</div>
{{end}}
</main>
{{if not .Off}}<script nonce="{{.Nonce}}">
(function () {
  var secure = "https://" + location.hostname + ":{{.HTTPSPort}}";
  var codeInput = document.getElementById("code");
  var go = document.getElementById("continue");
  var ua = navigator.userAgent;
  var which = /iPhone|iPad|iPod/.test(ua) || (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1) ? "ios"
    : /Android/.test(ua) ? "android" : /CrOS/.test(ua) ? "chromeos" : /Windows/.test(ua) ? "windows" : /Macintosh/.test(ua) ? "mac" : "";
  if (which) document.getElementById(which).open = true;
  var trusted = false;
  function update() {
    var code = codeInput.value.trim();
    go.hidden = !trusted || !code;
    go.href = secure + "/#join=" + encodeURIComponent(code);
  }
  function check() {
    if (trusted) return;
    fetch(secure + "/join/ping", { cache: "no-store" }).then(function (r) {
      if (!r.ok) return;
      trusted = true;
      document.getElementById("waiting").hidden = true;
      document.getElementById("trusted").hidden = false;
      document.getElementById("steps").hidden = true;
      update();
      if (codeInput.value.trim()) location.href = go.href;
    }).catch(function () {});
  }
  codeInput.addEventListener("input", update);
  document.addEventListener("visibilitychange", check);
  setInterval(check, 3000);
  check();
  update();
})();
</script>{{end}}
</body></html>
`))
