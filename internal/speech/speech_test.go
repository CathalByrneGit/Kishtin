package speech

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocal(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1": true, "localhost": true, "::1": true, "192.168.1.20": true, "10.0.0.5": true, "172.16.3.4": true,
		"fe80::1": true, "whisper": true, "kitchen-pc.local": true, "pi.lan": true, "speech.internal": true,
		"8.8.8.8": false, "api.openai.com": false, "example.com": false, "2001:4860:4860::8888": false, "": false,
	} {
		assert.Equal(t, want, Local(host), host)
	}
}

func TestNewRefusesCloudAddresses(t *testing.T) {
	_, err := New("https://api.example.com/v1", "", time.Second)
	assert.ErrorContains(t, err, "never leave the building")
	_, err = New("not a url", "", time.Second)
	assert.ErrorContains(t, err, "write it as")
	c, err := New("", "", time.Second)
	require.NoError(t, err)
	assert.False(t, c.CanListen())
	assert.False(t, c.CanSpeak())
	_, err = c.Transcribe(context.Background(), []byte("x"), "audio/webm", "")
	assert.ErrorIs(t, err, ErrNotSetUp)
}

func TestTranscribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/inference", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(1<<20))
		f, h, err := r.FormFile("file")
		require.NoError(t, err)
		b, _ := io.ReadAll(f)
		assert.Equal(t, "synthetic audio", string(b))
		assert.Equal(t, "speech.webm", h.Filename)
		assert.Equal(t, "Tables 4, 7", r.FormValue("prompt"))
		assert.Equal(t, "json", r.FormValue("response_format"))
		_ = json.NewEncoder(w).Encode(map[string]string{"text": " [BLANK_AUDIO] Fire table 4 mains. (kitchen noise)"})
	}))
	defer srv.Close()
	c, err := New(srv.URL, "", time.Second)
	require.NoError(t, err)
	text, err := c.Transcribe(context.Background(), []byte("synthetic audio"), "audio/webm;codecs=opus", "Tables 4, 7")
	require.NoError(t, err)
	assert.Equal(t, "Fire table 4 mains.", text)
}

func TestTranscribeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failed to read audio", http.StatusBadRequest)
	}))
	defer srv.Close()
	c, err := New(srv.URL, "", time.Second)
	require.NoError(t, err)
	_, err = c.Transcribe(context.Background(), []byte("x"), "audio/webm", "")
	assert.ErrorContains(t, err, "400 Bad Request: failed to read audio")
}

func TestSpeak(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&in))
		assert.Equal(t, "Table 4 mains fired.", in["text"])
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFF synthetic"))
	}))
	defer srv.Close()
	c, err := New("", srv.URL, time.Second)
	require.NoError(t, err)
	b, err := c.Speak(context.Background(), "Table 4 mains fired.")
	require.NoError(t, err)
	assert.Equal(t, "RIFF synthetic", string(b))
}
