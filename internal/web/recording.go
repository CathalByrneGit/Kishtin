package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

type rangeIn struct {
	From  string `query:"from" doc:"Start: a date (2026-05-01, Dublin) or an RFC 3339 time. Empty: from the beginning."`
	To    string `query:"to" doc:"End: a date (inclusive) or an RFC 3339 time (exclusive). Empty: up to now."`
	Limit int    `query:"limit" minimum:"0" doc:"At most this many; 0 for the most allowed."`
}

type activityIn struct {
	From  string   `query:"from" doc:"Start: a date (2026-05-01, Dublin) or an RFC 3339 time. Empty: from the beginning."`
	To    string   `query:"to" doc:"End: a date (inclusive) or an RFC 3339 time (exclusive). Empty: up to now."`
	Limit int      `query:"limit" minimum:"0" doc:"At most this many; 0 for the most allowed."`
	Kind  []string `query:"kind" doc:"Only these kinds, such as dish.sold_out or login."`
}

type snapshotsBody struct {
	Snapshots []service.PlanSnapshotView `json:"snapshots"`
}

type activityBody struct {
	Entries []store.Activity `json:"entries"`
}

type versionsBody struct {
	Versions []store.ImportedVersion `json:"versions"`
}

type audioOut struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

func (s *Server) recordingRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "plan-snapshots", Method: http.MethodGet, Path: "/kitchen/plans", Tags: []string{"Kitchen"},
		Summary:     "List the scheduler's plans as they changed",
		Description: "A snapshot is kept each time the plan changes: each station's recommendations and each fired course's status, with the reason and, for a course at risk or late, its cause (equipment_full, started_late, cooking_long, waiting_at_pass, busy). Head chef or manager.",
	}, wrap(s, func(ctx context.Context, in *rangeIn) (*out[snapshotsBody], error) {
		from, err := parseWhen(in.From, false)
		if err != nil {
			return nil, err
		}
		to, err := parseWhen(in.To, true)
		if err != nil {
			return nil, err
		}
		r, err := s.svc.PlanSnapshots(ctx, principal(ctx), from, to, in.Limit)
		return reply(snapshotsBody{Snapshots: r}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "list-activity", Method: http.MethodGet, Path: "/activity", Tags: []string{"Activity"},
		Summary:     "List the activity log, oldest first",
		Description: "Dishes sold out and back on, logins and logouts, screens connecting and dropping off, how each HACCP record was entered, and leavers anonymised. Each entry names the person and device.",
	}, wrap(s, func(ctx context.Context, in *activityIn) (*out[activityBody], error) {
		from, err := parseWhen(in.From, false)
		if err != nil {
			return nil, err
		}
		to, err := parseWhen(in.To, true)
		if err != nil {
			return nil, err
		}
		r, err := s.svc.Activity(ctx, principal(ctx), service.ActivityQuery{From: from, To: to, Kinds: in.Kind, Limit: in.Limit})
		return reply(activityBody{Entries: r}, err)
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "voice-recording", Method: http.MethodGet, Path: "/voice/log/{id}/audio", Tags: []string{"Voice"},
		Summary:     "Play a kept voice recording (manager password)",
		Description: "Recordings are kept only when system settings say to (voice_audio_days), and deleted after that many days.",
	}, wrap(s, func(ctx context.Context, in *struct {
		ID string `path:"id"`
	}) (*audioOut, error) {
		b, ct, err := s.svc.VoiceRecording(ctx, principal(ctx), in.ID)
		if err != nil {
			return nil, err
		}
		return &audioOut{ContentType: ct, Body: b}, nil
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "preview-anonymise", Method: http.MethodGet, Path: "/users/{id}/anonymise", Tags: []string{"Staff"},
		Summary: "Show what anonymising a leaver would change, without changing anything",
	}, wrap(s, func(ctx context.Context, in *struct {
		ID string `path:"id"`
	}) (*out[service.AnonymisePreview], error) {
		return reply(s.svc.PreviewAnonymise(ctx, principal(ctx), in.ID))
	}))
	huma.Register(s.api, huma.Operation{
		OperationID: "anonymise-user", Method: http.MethodPost, Path: "/users/{id}/anonymise", Tags: []string{"Staff"},
		Summary:     "Anonymise a leaver (manager password); cannot be undone",
		Description: "Replaces a deactivated member of staff's name with \"Former staff N\" in order events, the voice log, the activity log and the staff list, removes their PIN and password and revokes their API keys. HACCP records keep their name.",
	}, wrap(s, func(ctx context.Context, in *struct {
		ID string `path:"id"`
	}) (*out[service.AnonymiseResult], error) {
		return reply(s.svc.Anonymise(ctx, principal(ctx), in.ID))
	}))
}
