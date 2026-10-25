package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	c, err := New("", "", 0)
	require.NoError(t, err)
	assert.False(t, c.Enabled())
	_, err = New("https://api.example.com", "m", 0)
	assert.ErrorContains(t, err, "never leaves the building")
	_, err = New("127.0.0.1:8081", "m", 0)
	assert.ErrorContains(t, err, "write it as")
	c, err = New("http://127.0.0.1:8081", "m", time.Second)
	require.NoError(t, err)
	assert.True(t, c.Enabled())
}

func TestJSON(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + "```json\\n{\\\"columns\\\":[\\\"Date\\\"]}\\n```" + `"}}]}`))
	}))
	defer srv.Close()
	c, err := New(srv.URL, "lfm2-vl", 5*time.Second)
	require.NoError(t, err)
	var out struct {
		Columns []string `json:"columns"`
	}
	require.NoError(t, c.JSON(context.Background(), "You read forms.", "Which columns?", []Image{{MIME: "image/png", Data: []byte{1, 2}}}, &out))
	assert.Equal(t, []string{"Date"}, out.Columns)
	assert.Equal(t, "lfm2-vl", got["model"])
	msgs := got["messages"].([]any)
	user := msgs[1].(map[string]any)["content"].([]any)
	assert.Equal(t, "image_url", user[1].(map[string]any)["type"])
	assert.True(t, strings.HasPrefix(user[1].(map[string]any)["image_url"].(map[string]any)["url"].(string), "data:image/png;base64,"))

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Sure! Here are the columns."}}]}`))
	}))
	defer bad.Close()
	c, _ = New(bad.URL, "m", time.Second)
	assert.ErrorContains(t, c.JSON(context.Background(), "s", "p", nil, &out), "not the JSON asked for")

	var none *Client
	assert.ErrorIs(t, none.JSON(context.Background(), "s", "p", nil, &out), ErrNotSetUp)
}
