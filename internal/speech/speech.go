// Package speech talks to the local speech sidecars: a speech-to-text
// server (whisper.cpp's server) and a text-to-speech server (Piper's HTTP
// server). Both run on the restaurant's own machine or network. Audio and
// text never go to a cloud service: a sidecar address must be on this
// machine or the local network, or the client refuses to use it.
package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotSetUp is returned when a sidecar address is not configured.
var ErrNotSetUp = errors.New("not set up")

// Client calls the sidecars. A zero URL turns that half off.
type Client struct {
	STT  *url.URL
	TTS  *url.URL
	HTTP *http.Client
}

// New checks the sidecar addresses and returns a client. Empty addresses
// are allowed and turn that half off.
func New(sttURL, ttsURL string, timeout time.Duration) (*Client, error) {
	c := &Client{HTTP: &http.Client{Timeout: timeout}}
	var err error
	if c.STT, err = localURL("speech.stt_url", sttURL); err != nil {
		return nil, err
	}
	if c.TTS, err = localURL("speech.tts_url", ttsURL); err != nil {
		return nil, err
	}
	return c, nil
}

// localURL parses a sidecar address and checks it stays in the building.
func localURL(name, raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%s %q: write it as http://127.0.0.1:8178", name, raw)
	}
	if !Local(u.Hostname()) {
		return nil, fmt.Errorf("%s %q is not on this machine or the local network. Kitchen audio and transcripts never leave the building: run the sidecar locally", name, raw)
	}
	return u, nil
}

// Local reports whether a host is this machine or a private network
// address: loopback, private and link-local IPs, "localhost", a
// single-label name (such as a container named "whisper"), or a name under
// .local, .lan, .home.arpa or .internal.
func Local(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || !strings.Contains(host, ".") && net.ParseIP(host) == nil {
		return host != ""
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	for _, suffix := range []string{".local", ".lan", ".home.arpa", ".internal", ".localhost"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// CanListen reports whether speech-to-text is set up.
func (c *Client) CanListen() bool { return c != nil && c.STT != nil }

// CanSpeak reports whether text-to-speech is set up.
func (c *Client) CanSpeak() bool { return c != nil && c.TTS != nil }

// Transcribe sends audio to the speech-to-text server and returns what was
// said. prompt biases recognition toward kitchen words (the menu, open
// tables). It speaks whisper.cpp's server API: POST /inference with the
// audio as a multipart file. Start the server with --convert so it accepts
// the browser's WebM or Ogg audio.
func (c *Client) Transcribe(ctx context.Context, audio []byte, contentType, prompt string) (string, error) {
	if !c.CanListen() {
		return "", fmt.Errorf("speech-to-text: %w", ErrNotSetUp)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "speech"+extension(contentType))
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(audio); err != nil {
		return "", err
	}
	for k, v := range map[string]string{"response_format": "json", "temperature": "0", "language": "en", "prompt": prompt} {
		if err := mw.WriteField(k, v); err != nil {
			return "", err
		}
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.STT.JoinPath("inference").String(), &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("speech-to-text at %s: %w", c.STT.Host, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("speech-to-text at %s: %w", c.STT.Host, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("speech-to-text at %s answered %s: %s", c.STT.Host, resp.Status, strings.TrimSpace(string(b)))
	}
	var out struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("speech-to-text at %s: unexpected answer: %w", c.STT.Host, err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("speech-to-text at %s: %s", c.STT.Host, out.Error)
	}
	return cleanTranscript(out.Text), nil
}

// cleanTranscript drops the markers whisper writes for non-speech, such as
// "[BLANK_AUDIO]" or "(music)".
func cleanTranscript(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '[', '(':
			depth++
		case ']', ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func extension(contentType string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "webm"):
		return ".webm"
	case strings.Contains(ct, "ogg"):
		return ".ogg"
	case strings.Contains(ct, "mp4"), strings.Contains(ct, "aac"), strings.Contains(ct, "m4a"):
		return ".m4a"
	case strings.Contains(ct, "mpeg"), strings.Contains(ct, "mp3"):
		return ".mp3"
	}
	return ".wav"
}

// Speak asks the text-to-speech server to say text and returns WAV audio.
// It speaks Piper's HTTP server API: POST / with {"text": "..."}.
func (c *Client) Speak(ctx context.Context, text string) ([]byte, error) {
	if !c.CanSpeak() {
		return nil, fmt.Errorf("text-to-speech: %w", ErrNotSetUp)
	}
	b, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TTS.String(), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("text-to-speech at %s: %w", c.TTS.Host, err)
	}
	defer resp.Body.Close()
	audio, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, fmt.Errorf("text-to-speech at %s: %w", c.TTS.Host, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("text-to-speech at %s answered %s", c.TTS.Host, resp.Status)
	}
	return audio, nil
}
