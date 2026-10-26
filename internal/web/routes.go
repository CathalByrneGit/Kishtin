package web

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/service"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

func (s *Server) routes() {
	s.authRoutes()
	s.haccpRoutes()
	s.packRoute()
	s.bookRoutes()
	s.signOffRoutes()
	s.copyRoutes()
	s.catchUpRoutes()
	s.recordingRoutes()
	s.reportRoutes()
	s.limitRoutes()
	s.fileRoutes()
	s.settingsRoutes()
	s.adminRoutes()
	s.kitchenRoutes()
	s.voiceRoutes()
	s.squareRoutes()
	s.connectorRoutes()
}

// --- Auth ---

type staffOut struct {
	Body struct {
		Staff []service.StaffEntry `json:"staff"`
	}
}

type loginIn struct {
	Body struct {
		UserID   string `json:"user_id" minLength:"1"`
		PIN      string `json:"pin,omitempty"`
		Password string `json:"password,omitempty"`
	}
}

type loginOut struct {
	Body service.LoginResult
}

type meOut struct {
	Body struct {
		User   *auth.User  `json:"user,omitempty"`
		Method auth.Method `json:"method,omitempty"`
		Key    auth.APIKey `json:"key"`
		// Can lists what this request may do: yes, password (after a
		// password login) or no. The server checks every action again.
		Can map[auth.Action]service.Allowed `json:"can"`
	}
}

type actionsOut struct {
	Body []auth.ActionInfo
}

type empty struct{}

func (s *Server) authRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "list-staff", Method: http.MethodGet, Path: "/auth/staff", Tags: []string{"Auth"},
		Summary: "List staff for the login screen",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*staffOut, error) {
		staff, err := s.svc.StaffList(ctx)
		if err != nil {
			return nil, err
		}
		out := &staffOut{}
		out.Body.Staff = staff
		return out, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "login", Method: http.MethodPost, Path: "/auth/login", Tags: []string{"Auth"},
		Summary:     "Log in on a kitchen device",
		Description: "Send a PIN for a 12-hour staff session, or a manager password for a 30-minute session that allows manager actions. Needs a device key.",
	}, wrap(s, func(ctx context.Context, in *loginIn) (*loginOut, error) {
		var res service.LoginResult
		var err error
		switch {
		case in.Body.PIN != "" && in.Body.Password != "":
			return nil, huma.Error422UnprocessableEntity("send a PIN or a password, not both")
		case in.Body.Password != "":
			res, err = s.svc.LoginPassword(ctx, caller(ctx), in.Body.UserID, in.Body.Password)
		case in.Body.PIN != "":
			res, err = s.svc.LoginPIN(ctx, caller(ctx), in.Body.UserID, in.Body.PIN)
		default:
			return nil, huma.Error422UnprocessableEntity("send a PIN or a password")
		}
		if err != nil {
			return nil, err
		}
		return &loginOut{Body: res}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "logout", Method: http.MethodPost, Path: "/auth/logout", Tags: []string{"Auth"},
		Summary: "End the staff session sent in " + SessionHeader, DefaultStatus: http.StatusNoContent,
	}, wrap(s, func(ctx context.Context, in *struct {
		Session string `header:"X-Kishtin-Session"`
	}) (*empty, error) {
		return &empty{}, s.svc.Logout(ctx, caller(ctx), in.Session)
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "me", Method: http.MethodGet, Path: "/auth/me", Tags: []string{"Auth"},
		Summary: "Show who this request is from",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*meOut, error) {
		c := caller(ctx)
		out := &meOut{}
		out.Body.Key = c.Key
		if c.Principal != nil {
			u := c.Principal.User
			out.Body.User, out.Body.Method = &u, c.Principal.Method
		}
		out.Body.Can = s.svc.Can(c.Principal)
		return out, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "list-actions", Method: http.MethodGet, Path: "/auth/actions", Tags: []string{"Auth"},
		Summary:     "List every action and who may take it by default",
		Description: "The actions permissions are set for, in screen order, with Kishtin's default roles and whether a PIN is enough. Fixed actions are always managers with a password.",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*actionsOut, error) {
		a, err := s.svc.PermissionActions(caller(ctx).Principal)
		if err != nil {
			return nil, err
		}
		return &actionsOut{Body: a}, nil
	}))
}

// --- HACCP records ---

type recordIn struct {
	Body service.NewRecord
}

type recordOut struct {
	Body haccp.Record
}

type checkOut struct {
	Body service.Check
}

type todayOut struct {
	Body service.TodayView
}

type listRecordsIn struct {
	From     string   `query:"from" doc:"Start, inclusive: a date (2026-05-01, Dublin time) or an RFC 3339 time."`
	To       string   `query:"to" doc:"End: a date (inclusive, Dublin time) or an RFC 3339 time (exclusive)."`
	Types    []string `query:"type" doc:"Only these record types."`
	Outcomes []string `query:"outcome" doc:"Only these outcomes: pass, borderline, fail, recorded."`
	Limit    int      `query:"limit" minimum:"0" maximum:"1000" doc:"At most this many records (default and maximum 1000)."`
	Newest   bool     `query:"newest" doc:"Newest first."`
}

type recordsOut struct {
	Body struct {
		Records []haccp.Record `json:"records"`
	}
}

type verifyOut struct {
	Body service.VerifyReport
}

type exportIn struct {
	From     string   `query:"from" doc:"Start date (2026-05-01, Dublin time) or RFC 3339 time. Empty: from the first record."`
	To       string   `query:"to" doc:"End date, inclusive, or RFC 3339 time, exclusive. Empty: up to now."`
	Types    []string `query:"type" doc:"Only these record types."`
	Outcomes []string `query:"outcome" doc:"Only these outcomes: pass, borderline, fail, recorded."`
}

// query builds a record query from request parameters.
func query(fromS, toS string, types, outcomes []string) (service.RecordQuery, error) {
	from, err := parseWhen(fromS, false)
	if err != nil {
		return service.RecordQuery{}, err
	}
	to, err := parseWhen(toS, true)
	if err != nil {
		return service.RecordQuery{}, err
	}
	q := service.RecordQuery{From: from, To: to}
	for _, t := range types {
		q.Types = append(q.Types, haccp.RecordType(t))
	}
	for _, o := range outcomes {
		q.Outcomes = append(q.Outcomes, haccp.Outcome(o))
	}
	return q, nil
}

type csvOut struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	Records            int    `header:"X-Kishtin-Records"`
	ChainHead          string `header:"X-Kishtin-Chain-Head"`
	ChainOK            bool   `header:"X-Kishtin-Chain-OK"`
	Body               []byte
}

// parseWhen reads a date (Dublin time) or an RFC 3339 time. A date used as
// an end means the end of that day.
func parseWhen(v string, end bool) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t.UTC(), nil
	}
	d, err := time.ParseInLocation(haccp.DateLayout, v, clock.Dublin)
	if err != nil {
		return time.Time{}, huma.Error422UnprocessableEntity(fmt.Sprintf("%q is not a date like 2026-05-01 or a time like 2026-05-01T09:00:00Z", v))
	}
	if end {
		d = d.AddDate(0, 0, 1)
	}
	return d.UTC(), nil
}

func (s *Server) haccpRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "today", Method: http.MethodGet, Path: "/haccp/today", Tags: []string{"HACCP"},
		Summary:     "Show today's checks",
		Description: "Each fridge and freezer check (done, due or missed), the cleaning checklist, the share of checks done, today's out-of-limit records and batches in use.",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*todayOut, error) {
		t, err := s.svc.Today(ctx, principal(ctx))
		if err != nil {
			return nil, err
		}
		return &todayOut{Body: t}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "check-record", Method: http.MethodPost, Path: "/haccp/check", Tags: []string{"HACCP"},
		Summary:     "Check a reading without saving it",
		Description: "Returns the pass, borderline or fail result and the limit it was checked against, so it can be shown before saving.",
	}, wrap(s, func(ctx context.Context, in *recordIn) (*checkOut, error) {
		c, err := s.svc.CheckRecord(ctx, principal(ctx), in.Body)
		if err != nil {
			return nil, err
		}
		return &checkOut{Body: c}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "log-record", Method: http.MethodPost, Path: "/haccp/records", Tags: []string{"HACCP"},
		Summary:       "Save a HACCP record",
		Description:   "A failed or borderline reading is refused unless it has a corrective action. Records cannot be changed later: to fix a mistake, save a new record with `corrects` set.",
		DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, in *recordIn) (*recordOut, error) {
		r, err := s.svc.LogRecord(ctx, principal(ctx), in.Body)
		if err != nil {
			return nil, err
		}
		return &recordOut{Body: r}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "list-records", Method: http.MethodGet, Path: "/haccp/records", Tags: []string{"HACCP"},
		Summary: "List HACCP records",
	}, wrap(s, func(ctx context.Context, in *listRecordsIn) (*recordsOut, error) {
		q, err := query(in.From, in.To, in.Types, in.Outcomes)
		if err != nil {
			return nil, err
		}
		q.Limit, q.Newest = in.Limit, in.Newest
		recs, err := s.svc.ListRecords(ctx, principal(ctx), q)
		if err != nil {
			return nil, err
		}
		out := &recordsOut{}
		out.Body.Records = recs
		if out.Body.Records == nil {
			out.Body.Records = []haccp.Record{}
		}
		return out, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "get-record", Method: http.MethodGet, Path: "/haccp/records/{id}", Tags: []string{"HACCP"},
		Summary: "Get one HACCP record",
	}, wrap(s, func(ctx context.Context, in *struct {
		ID string `path:"id"`
	}) (*recordOut, error) {
		r, err := s.svc.GetRecord(ctx, principal(ctx), in.ID)
		if err != nil {
			return nil, err
		}
		return &recordOut{Body: r}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "verify", Method: http.MethodGet, Path: "/haccp/verify", Tags: []string{"HACCP"},
		Summary:     "Check HACCP records and files for tampering",
		Description: "Re-checks the hash chain from the first record, the limit versions records refer to, and every attached file.",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*verifyOut, error) {
		rep, err := s.svc.Verify(ctx, principal(ctx))
		if err != nil {
			return nil, err
		}
		return &verifyOut{Body: rep}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "export-records", Method: http.MethodGet, Path: "/haccp/export", Tags: []string{"HACCP"},
		Summary:     "Export HACCP records as CSV for an inspection",
		Description: "Takes the same filters as listing records. Rows are oldest first and carry record IDs and hashes, so the export can be checked against kishtin haccp verify.",
		Responses: map[string]*huma.Response{"200": {
			Description: "CSV of the records in the range.",
			Content:     map[string]*huma.MediaType{"text/csv": {Schema: &huma.Schema{Type: "string"}}},
		}},
	}, wrap(s, func(ctx context.Context, in *exportIn) (*csvOut, error) {
		q, err := query(in.From, in.To, in.Types, in.Outcomes)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		sum, err := s.svc.ExportRecordsCSV(ctx, principal(ctx), q, &buf)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("haccp-records-%s.csv", time.Now().In(clock.Dublin).Format(haccp.DateLayout))
		return &csvOut{
			ContentType:        "text/csv; charset=utf-8",
			ContentDisposition: fmt.Sprintf("attachment; filename=%q", safeFilename(name)),
			Records:            sum.Records,
			ChainHead:          sum.Chain.HeadHash,
			ChainOK:            sum.Chain.OK,
			Body:               buf.Bytes(),
		}, nil
	}))
}

type pdfOut struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	Records            int    `header:"X-Kishtin-Records"`
	ChainHead          string `header:"X-Kishtin-Chain-Head"`
	ChainOK            bool   `header:"X-Kishtin-Chain-OK"`
	Body               []byte
}

func (s *Server) packRoute() {
	huma.Register(s.api, huma.Operation{
		OperationID: "export-pack", Method: http.MethodGet, Path: "/haccp/export/pdf", Tags: []string{"HACCP"},
		Summary:     "Export the HACCP inspection pack as PDF",
		Description: "Takes the same filters as the CSV export. The pack has a summary with the hash chain and file checks, failed and borderline records with their actions, every record by day, and the limit versions used.",
		Responses: map[string]*huma.Response{"200": {
			Description: "The inspection pack.",
			Content:     map[string]*huma.MediaType{"application/pdf": {Schema: &huma.Schema{Type: "string", Format: "binary"}}},
		}},
	}, wrap(s, func(ctx context.Context, in *exportIn) (*pdfOut, error) {
		q, err := query(in.From, in.To, in.Types, in.Outcomes)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		sum, err := s.svc.ExportPackPDF(ctx, principal(ctx), q, &buf)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("haccp-inspection-pack-%s.pdf", time.Now().In(clock.Dublin).Format(haccp.DateLayout))
		return &pdfOut{
			ContentType:        "application/pdf",
			ContentDisposition: fmt.Sprintf("attachment; filename=%q", name),
			Records:            sum.Records,
			ChainHead:          sum.Chain.HeadHash,
			ChainOK:            sum.Chain.OK,
			Body:               buf.Bytes(),
		}, nil
	}))
}

func safeFilename(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.', r == '_':
			return r
		}
		return '_'
	}, s)
}

// --- Limits ---

type limitsOut struct {
	Body service.Limits
}

type newLimitIn struct {
	Body service.NewLimit
}

type limitPreviewOut struct {
	Body service.LimitPreview
}

type limitOut struct {
	Body haccp.LimitVersion
}

func (s *Server) limitRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "list-limits", Method: http.MethodGet, Path: "/haccp/limits", Tags: []string{"Limits"},
		Summary: "List critical limits: the ones in force and the full history",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*limitsOut, error) {
		l, err := s.svc.ListLimits(ctx, principal(ctx))
		if err != nil {
			return nil, err
		}
		return &limitsOut{Body: l}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "preview-limit", Method: http.MethodPost, Path: "/haccp/limits/preview", Tags: []string{"Limits"},
		Summary:     "Preview a limit change",
		Description: "Shows how recent records would have scored under the new limit, whether it is below FSAI guidance, and anything that stops it being saved. Saved records are never re-graded.",
	}, wrap(s, func(ctx context.Context, in *newLimitIn) (*limitPreviewOut, error) {
		pv, err := s.svc.PreviewLimit(ctx, principal(ctx), in.Body)
		if err != nil {
			return nil, err
		}
		return &limitPreviewOut{Body: pv}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "create-limit", Method: http.MethodPost, Path: "/haccp/limits", Tags: []string{"Limits"},
		Summary:       "Save a new limit version",
		Description:   "Needs a manager password login or a manager API key. A limit below FSAI guidance needs a reason and a HACCP plan reference.",
		DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, in *newLimitIn) (*limitOut, error) {
		lv, err := s.svc.CreateLimit(ctx, principal(ctx), in.Body)
		if err != nil {
			return nil, err
		}
		return &limitOut{Body: lv}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "export-limits", Method: http.MethodGet, Path: "/haccp/limits/export", Tags: []string{"Limits"},
		Summary: "Export the full limit history as CSV",
		Responses: map[string]*huma.Response{"200": {
			Description: "CSV of every limit version.",
			Content:     map[string]*huma.MediaType{"text/csv": {Schema: &huma.Schema{Type: "string"}}},
		}},
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*csvOut, error) {
		var buf bytes.Buffer
		if err := s.svc.ExportLimitsCSV(ctx, principal(ctx), &buf); err != nil {
			return nil, err
		}
		return &csvOut{
			ContentType:        "text/csv; charset=utf-8",
			ContentDisposition: `attachment; filename="kishtin-limits.csv"`,
			Body:               buf.Bytes(),
		}, nil
	}))
}

// --- Files ---

type uploadIn struct {
	Name    string `query:"name" doc:"Original file name."`
	RawBody []byte `contentType:"application/octet-stream"`
}

type fileOut struct {
	Body store.FileMeta
}

func (s *Server) fileRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "upload-file", Method: http.MethodPost, Path: "/files", Tags: []string{"Files"},
		Summary:       "Upload a docket or invoice photo",
		Description:   "Send the file as the request body. JPEG, PNG, WebP, HEIC and PDF are accepted. Attach it to a delivery record with its sha256.",
		DefaultStatus: http.StatusCreated,
		// The system settings set the real limit; this is the ceiling.
		MaxBodyBytes: settings.MaxUploadMBLimit << 20,
	}, wrap(s, func(ctx context.Context, in *uploadIn) (*fileOut, error) {
		m, err := s.svc.UploadFile(ctx, principal(ctx), bytes.NewReader(in.RawBody), in.Name)
		if err != nil {
			return nil, err
		}
		return &fileOut{Body: m}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "get-file", Method: http.MethodGet, Path: "/files/{sha256}", Tags: []string{"Files"},
		Summary: "Download an attached file",
	}, func(ctx context.Context, in *struct {
		SHA256 string `path:"sha256"`
	}) (*huma.StreamResponse, error) {
		m, f, err := s.svc.OpenFile(ctx, principal(ctx), in.SHA256)
		if err != nil {
			return nil, apiError(err)
		}
		return &huma.StreamResponse{Body: func(hc huma.Context) {
			defer f.Close()
			hc.SetHeader("Content-Type", m.MIME)
			hc.SetHeader("Content-Length", fmt.Sprint(m.Size))
			hc.SetHeader("Content-Disposition", fmt.Sprintf("inline; filename=%q", safeFilename(m.OriginalName)))
			if _, err := io.Copy(hc.BodyWriter(), f); err != nil {
				s.log.Warn("send file", "sha256", m.SHA256, "err", err)
			}
		}}, nil
	})
}

// --- Staff, keys and backups ---

type usersOut struct {
	Body struct {
		Users []auth.User `json:"users"`
	}
}

type newUserIn struct {
	Body service.NewUser
}

type userOut struct {
	Body auth.User
}

type updateUserIn struct {
	ID   string `path:"id"`
	Body service.UserChange
}

type newKeyIn struct {
	Body struct {
		Kind   auth.KeyKind `json:"kind" enum:"device,user"`
		Name   string       `json:"name" minLength:"1"`
		UserID string       `json:"user_id,omitempty" doc:"For a user key: who it acts as. Defaults to you."`
	}
}

type newKeyOut struct {
	Body service.NewKeyResult
}

type keysOut struct {
	Body struct {
		Keys []auth.APIKey `json:"keys"`
	}
}

type backupOut struct {
	Body service.Backup
}

type backupsOut struct {
	Body struct {
		Backups []service.Backup `json:"backups"`
	}
}

func (s *Server) adminRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "list-users", Method: http.MethodGet, Path: "/users", Tags: []string{"Staff"},
		Summary: "List staff, including deactivated staff",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*usersOut, error) {
		users, err := s.svc.ListUsers(ctx, principal(ctx))
		if err != nil {
			return nil, err
		}
		out := &usersOut{}
		out.Body.Users = users
		return out, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "create-user", Method: http.MethodPost, Path: "/users", Tags: []string{"Staff"},
		Summary: "Add a staff member", DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, in *newUserIn) (*userOut, error) {
		u, err := s.svc.CreateUser(ctx, principal(ctx), in.Body)
		if err != nil {
			return nil, err
		}
		return &userOut{Body: u}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "update-user", Method: http.MethodPatch, Path: "/users/{id}", Tags: []string{"Staff"},
		Summary: "Change a staff member: name, role, PIN, password, or deactivate",
	}, wrap(s, func(ctx context.Context, in *updateUserIn) (*userOut, error) {
		u, err := s.svc.UpdateUser(ctx, principal(ctx), in.ID, in.Body)
		if err != nil {
			return nil, err
		}
		return &userOut{Body: u}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "create-key", Method: http.MethodPost, Path: "/keys", Tags: []string{"Keys"},
		Summary:       "Create an API key",
		Description:   "A device key lets a shared kitchen device show the login screen. A user key acts as one person, for the CLI and scripts. The secret is shown once.",
		DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, in *newKeyIn) (*newKeyOut, error) {
		res, err := s.svc.CreateKey(ctx, principal(ctx), in.Body.Kind, in.Body.Name, in.Body.UserID)
		if err != nil {
			return nil, err
		}
		return &newKeyOut{Body: res}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "list-keys", Method: http.MethodGet, Path: "/keys", Tags: []string{"Keys"},
		Summary: "List API keys",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*keysOut, error) {
		keys, err := s.svc.ListKeys(ctx, principal(ctx))
		if err != nil {
			return nil, err
		}
		out := &keysOut{}
		out.Body.Keys = keys
		return out, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "revoke-key", Method: http.MethodDelete, Path: "/keys/{id}", Tags: []string{"Keys"},
		Summary: "Revoke an API key", DefaultStatus: http.StatusNoContent,
	}, wrap(s, func(ctx context.Context, in *struct {
		ID string `path:"id"`
	}) (*empty, error) {
		return &empty{}, s.svc.RevokeKey(ctx, principal(ctx), in.ID)
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "create-backup", Method: http.MethodPost, Path: "/backups", Tags: []string{"Backups"},
		Summary:       "Back up the database now",
		Description:   "Writes a consistent copy, verifies its HACCP chain, and writes a .sha256 file next to it. Copy backups off the machine yourself; Kishtin does not send them anywhere.",
		DefaultStatus: http.StatusCreated,
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*backupOut, error) {
		b, err := s.svc.BackupNow(ctx, principal(ctx))
		if err != nil {
			return nil, err
		}
		return &backupOut{Body: b}, nil
	}))

	huma.Register(s.api, huma.Operation{
		OperationID: "list-backups", Method: http.MethodGet, Path: "/backups", Tags: []string{"Backups"},
		Summary: "List backups, newest first",
	}, wrap(s, func(ctx context.Context, _ *struct{}) (*backupsOut, error) {
		bs, err := s.svc.ListBackups(ctx, principal(ctx))
		if err != nil {
			return nil, err
		}
		out := &backupsOut{}
		out.Body.Backups = bs
		return out, nil
	}))
}
