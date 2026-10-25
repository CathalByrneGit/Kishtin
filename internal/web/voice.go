package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
)

type voiceCommandBody struct {
	// Text is the command, as it would be said: "fire table 4 mains".
	Text string `json:"text" minLength:"1" maxLength:"300"`
}

type voiceAudioIn struct {
	ContentType string `header:"Content-Type" doc:"The recording's type, such as audio/webm or audio/wav."`
	RawBody     []byte `contentType:"audio/webm"`
}

type speakBody struct {
	Text string `json:"text" minLength:"1" maxLength:"500"`
}

type wavOut struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

type voiceLogBody struct {
	Entries []voice.LogEntry `json:"entries"`
}

func (s *Server) voiceRoutes() {
	tags := []string{"Voice"}
	huma.Register(s.api, huma.Operation{
		OperationID: "voice-status", Method: http.MethodGet, Path: "/voice", Tags: tags,
		Summary: "Show whether spoken commands and spoken replies are set up, and any question waiting for an answer",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*out[service.VoiceStatus], error) {
		return reply(s.svc.VoiceStatus(ctx, principal(ctx)))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "voice-command", Method: http.MethodPost, Path: "/voice/command", Tags: tags,
		Summary:     "Run a typed voice command",
		Description: "The same grammar as spoken commands, such as \"fire table 4 mains\" or \"walk-in fridge 3.4 degrees\". When the reply is waiting, the next command answers it (a choice, a corrective action, or \"confirm\"). Every command is logged with what was said.",
	}, wrap(s, func(ctx context.Context, in *out[voiceCommandBody]) (*out[service.VoiceReply], error) {
		return reply(s.svc.VoiceCommand(ctx, principal(ctx), in.Body.Text))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "voice-audio", Method: http.MethodPost, Path: "/voice/audio", Tags: tags,
		Summary:      "Run a spoken command",
		Description:  "Send push-to-talk audio as the request body. The server transcribes it with the local speech-to-text sidecar (no audio leaves the building), then runs it like a typed command. The audio is not stored; the transcript is logged.",
		MaxBodyBytes: 4 << 20,
	}, wrap(s, func(ctx context.Context, in *voiceAudioIn) (*out[service.VoiceReply], error) {
		return reply(s.svc.VoiceAudio(ctx, principal(ctx), in.RawBody, in.ContentType))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "voice-speak", Method: http.MethodPost, Path: "/voice/speak", Tags: tags,
		Summary: "Say a reply with the local text-to-speech sidecar; returns WAV audio",
	}, wrap(s, func(ctx context.Context, in *out[speakBody]) (*wavOut, error) {
		b, err := s.svc.VoiceSpeak(ctx, principal(ctx), in.Body.Text)
		if err != nil {
			return nil, err
		}
		return &wavOut{ContentType: "audio/wav", Body: b}, nil
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "voice-log", Method: http.MethodGet, Path: "/voice/log", Tags: tags,
		Summary: "List recent voice commands: what was heard, what it was taken to mean, and what happened",
	}, wrap(s, func(ctx context.Context, in *struct {
		Limit int `query:"limit" minimum:"0" maximum:"500" doc:"How many, newest first (default 100)."`
	}) (*out[voiceLogBody], error) {
		list, err := s.svc.VoiceLog(ctx, principal(ctx), in.Limit)
		return reply(voiceLogBody{Entries: list}, err)
	}))
}
