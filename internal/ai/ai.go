// Package ai talks to a small local model server, such as llama.cpp's
// llama-server or Ollama, through the OpenAI-compatible chat API. It is
// optional: Kishtin works without it. The server must be on this machine
// or the local network, so kitchen data never leaves the building. A model
// only suggests: everything it returns is a draft that a person reviews,
// and the same rules check it as anything typed.
package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/speech"
)

// Client is a local model server.
type Client struct {
	URL   *url.URL
	Model string
	HTTP  *http.Client
}

// New returns a client for a local model server, or nil when url is empty.
func New(rawURL, model string, timeout time.Duration) (*Client, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("ai.url %q: write it as http://127.0.0.1:8081", rawURL)
	}
	if !speech.Local(u.Hostname()) {
		return nil, fmt.Errorf("ai.url %q is not on this machine or the local network. Kitchen data never leaves the building: run the model locally", rawURL)
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &Client{URL: u, Model: model, HTTP: &http.Client{Timeout: timeout}}, nil
}

// Enabled reports whether a model server is set up.
func (c *Client) Enabled() bool { return c != nil && c.URL != nil }

// Image is a picture sent with a request, such as a photo of a paper page.
type Image struct {
	MIME string
	Data []byte
}

type part struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// ErrNotSetUp means no model server is configured.
var ErrNotSetUp = errors.New("no local model is set up: add [ai] url to kishtin.toml")

// JSON asks the model for a JSON answer and decodes it into out. The
// caller checks the result: a model can be wrong.
func (c *Client) JSON(ctx context.Context, system, prompt string, images []Image, out any) error {
	if !c.Enabled() {
		return ErrNotSetUp
	}
	var content any = prompt
	if len(images) > 0 {
		parts := []part{{Type: "text", Text: prompt}}
		for _, im := range images {
			parts = append(parts, part{Type: "image_url", ImageURL: &imageURL{URL: "data:" + im.MIME + ";base64," + base64.StdEncoding.EncodeToString(im.Data)}})
		}
		content = parts
	}
	body, err := json.Marshal(map[string]any{
		"model":           c.Model,
		"messages":        []message{{Role: "system", Content: system}, {Role: "user", Content: content}},
		"temperature":     0,
		"response_format": map[string]string{"type": "json_object"},
	})
	if err != nil {
		return err
	}
	endpoint := c.URL.JoinPath("v1", "chat", "completions")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("ask the local model: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read the local model's answer: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("the local model answered %s: %s", res.Status, strings.TrimSpace(string(raw[:min(len(raw), 300)])))
	}
	var reply struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil || len(reply.Choices) == 0 {
		return errors.New("the local model's answer is not a chat completion")
	}
	text := strings.TrimSpace(reply.Choices[0].Message.Content)
	text = strings.TrimPrefix(strings.TrimPrefix(text, "```json"), "```")
	text = strings.TrimSpace(strings.TrimSuffix(text, "```"))
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("the local model's answer is not the JSON asked for: %w", err)
	}
	return nil
}
