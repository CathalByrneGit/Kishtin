package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// Limits is the limit history and the versions in force now.
type Limits struct {
	Current []haccp.LimitVersion `json:"current"`
	History []haccp.LimitVersion `json:"history"`
}

// ListLimits returns every limit version and the current ones.
func (s *Service) ListLimits(ctx context.Context, p *auth.Principal) (Limits, error) {
	if err := allow(p, auth.ActViewHACCP); err != nil {
		return Limits{}, err
	}
	history, err := s.st.HACCP().ListLimitVersions(ctx)
	if err != nil {
		return Limits{}, err
	}
	return Limits{Current: haccp.Current(history), History: history}, nil
}

// NewLimit is a proposed limit version. Only the fields for its check type
// are set.
type NewLimit struct {
	Check          haccp.CheckType     `json:"check"`
	Scope          haccp.Scope         `json:"scope"`
	MaxC           *float64            `json:"max_c,omitempty"`
	BorderlineMaxC *float64            `json:"borderline_max_c,omitempty"`
	MinC           *float64            `json:"min_c,omitempty"`
	Combos         []haccp.TimeTemp    `json:"combos,omitempty"`
	Cooling        *haccp.CoolingLimit `json:"cooling,omitempty"`
	NonFood        bool                `json:"non_food,omitempty"`
	Reason         string              `json:"reason"`
	PlanRef        string              `json:"plan_ref,omitempty"`
}

func (nl NewLimit) thresholds() haccp.Thresholds {
	return haccp.Thresholds{
		MaxC: nl.MaxC, BorderlineMaxC: nl.BorderlineMaxC, MinC: nl.MinC,
		Combos: nl.Combos, Cooling: nl.Cooling,
	}.Rounded()
}

// head returns the current version for exactly this check and scope.
func head(history []haccp.LimitVersion, check haccp.CheckType, scope haccp.Scope) *haccp.LimitVersion {
	for _, lv := range haccp.Current(history) {
		if lv.Check == check && lv.Scope == scope {
			return &lv
		}
	}
	return nil
}

// RecordRegrade shows how a recent record would have scored under a
// proposed limit. Saved records are never re-graded.
type RecordRegrade struct {
	RecordID       string        `json:"record_id"`
	LoggedAt       time.Time     `json:"logged_at"`
	Subject        string        `json:"subject"`
	Saved          haccp.Outcome `json:"saved"`
	Proposed       haccp.Outcome `json:"proposed"`
	Changed        bool          `json:"changed"`
	ProposedReason string        `json:"proposed_reason"`
}

// LimitPreview is what a limit change would do, shown before it is saved.
type LimitPreview struct {
	Limit haccp.LimitVersion `json:"limit"`
	// Replaces is the version the change supersedes, if any.
	Replaces *haccp.LimitVersion `json:"replaces,omitempty"`
	// Guidance names the reference guidance the limit is measured against.
	Guidance string `json:"guidance"`
	// BelowGuidance lists how the limit is less strict than the guidance.
	// Saving it needs a HACCP plan reference.
	BelowGuidance []string `json:"below_guidance,omitempty"`
	// BelowFSAI lists how the limit is less strict than the FSAI defaults,
	// whatever the guidance says.
	BelowFSAI []string        `json:"below_fsai,omitempty"`
	Recent    []RecordRegrade `json:"recent"`
	Changed   int             `json:"changed"`
	// Problem is set when the limit cannot be saved as it is.
	Problem string `json:"problem,omitempty"`
}

// buildLimit turns a proposed limit into a version, read through st: the
// scope name is matched to the lists, and the limit is checked against the
// current guidance. A problem that stops it being saved is returned in
// problem, not err.
func (s *Service) buildLimit(ctx context.Context, st store.Store, p *auth.Principal, id string, nl NewLimit) (lv haccp.LimitVersion, pv LimitPreview, problem string, err error) {
	history, err := st.HACCP().ListLimitVersions(ctx)
	if err != nil {
		return lv, pv, "", err
	}
	guidance, err := s.Guidance().current(ctx, st)
	if err != nil {
		return lv, pv, "", err
	}
	rules, err := s.Rules().current(ctx, st)
	if err != nil {
		return lv, pv, "", err
	}
	lists, err := s.Lists().current(ctx, st)
	if err != nil {
		return lv, pv, "", err
	}

	lv = haccp.LimitVersion{
		ID: id, Check: nl.Check,
		Scope:             haccp.Scope{Kind: nl.Scope.Kind, Ref: strings.TrimSpace(nl.Scope.Ref)},
		Thresholds:        nl.thresholds(),
		NonFood:           nl.NonFood,
		GuidanceVersionID: guidance.Version.ID,
		CreatedBy:         p.User.ID,
		CreatedAt:         s.clk.Now(),
		Reason:            strings.TrimSpace(nl.Reason),
		PlanRef:           strings.TrimSpace(nl.PlanRef),
	}
	pv = LimitPreview{Guidance: guidance.Value.Name, Recent: []RecordRegrade{}}
	if lv.Scope, err = lists.Value.ApplyScope(lv.Check, lv.Scope, rules.Value); err != nil {
		pv.Limit = lv
		return lv, pv, err.Error(), nil
	}
	if h := head(history, lv.Check, lv.Scope); h != nil {
		pv.Replaces, lv.Supersedes = h, h.ID
	}
	pv.Limit = lv
	if err := lv.ValidateShape(); err != nil {
		return lv, pv, err.Error(), nil
	}
	pv.BelowGuidance = lv.LooserThanGuidance(guidance.Value)
	pv.BelowFSAI = lv.LooserThanFSAI()
	if err := lv.ValidateNew(guidance.Value); err != nil {
		return lv, pv, err.Error(), nil
	}
	return lv, pv, "", nil
}

// PreviewLimit shows how recent records would have scored under a proposed
// limit, and anything that stops it being saved.
func (s *Service) PreviewLimit(ctx context.Context, p *auth.Principal, nl NewLimit) (LimitPreview, error) {
	if err := allow(p, auth.ActManageLimits); err != nil {
		return LimitPreview{}, err
	}
	lv, pv, problem, err := s.buildLimit(ctx, s.st, p, "(preview)", nl)
	if err != nil {
		return LimitPreview{}, err
	}
	pv.Problem = problem
	if lv.ValidateShape() != nil {
		return pv, nil
	}
	sys, err := s.System().current(ctx, s.st)
	if err != nil {
		return LimitPreview{}, err
	}
	history, err := s.st.HACCP().ListLimitVersions(ctx)
	if err != nil {
		return LimitPreview{}, err
	}
	recs, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{Types: recordTypesFor(lv.Check), Newest: true, Limit: 500})
	if err != nil {
		return LimitPreview{}, err
	}
	// The proposed limit only affects records it would apply to: those whose
	// most specific matching scope, after the change, is this one.
	proposed := append(slices.Clone(history), lv)
	for _, r := range recs {
		if len(pv.Recent) == sys.Value.PreviewRecords {
			break
		}
		check, ok := haccp.CheckFor(r.Type, r.Reading)
		if !ok || check != lv.Check {
			continue
		}
		if haccp.Resolve(proposed, check, haccp.Scopes(r.Type, r.Reading)).ID != lv.ID {
			continue
		}
		res, err := haccp.Evaluate(r.Type, r.Reading, &lv)
		if err != nil {
			continue
		}
		rg := RecordRegrade{
			RecordID: r.ID, LoggedAt: r.LoggedAt, Subject: subject(r),
			Saved: r.Outcome, Proposed: res.Outcome, Changed: res.Outcome != r.Outcome, ProposedReason: res.Reason,
		}
		if rg.Changed {
			pv.Changed++
		}
		pv.Recent = append(pv.Recent, rg)
	}
	return pv, nil
}

func recordTypesFor(c haccp.CheckType) []haccp.RecordType {
	switch c {
	case haccp.CheckFridge:
		return []haccp.RecordType{haccp.RecordFridge}
	case haccp.CheckFreezer:
		return []haccp.RecordType{haccp.RecordFreezer}
	case haccp.CheckCook:
		return []haccp.RecordType{haccp.RecordCook}
	case haccp.CheckReheat:
		return []haccp.RecordType{haccp.RecordReheat}
	case haccp.CheckHotHold:
		return []haccp.RecordType{haccp.RecordHotHold}
	case haccp.CheckCooling:
		return []haccp.RecordType{haccp.RecordCooling}
	default:
		return []haccp.RecordType{haccp.RecordDelivery}
	}
}

// CreateLimit saves a new limit version. It supersedes the current version
// for the same check and scope, if there is one.
func (s *Service) CreateLimit(ctx context.Context, p *auth.Principal, nl NewLimit) (haccp.LimitVersion, error) {
	if err := allow(p, auth.ActManageLimits); err != nil {
		return haccp.LimitVersion{}, err
	}
	var saved haccp.LimitVersion
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		lv, _, problem, err := s.buildLimit(ctx, tx, p, newID(), nl)
		if err != nil {
			return err
		}
		if problem != "" {
			return newErr(KindInvalid, "%s", problem)
		}
		if err := tx.HACCP().AppendLimitVersion(ctx, lv); err != nil {
			return mapStoreErr(err, "a newer version of this limit")
		}
		saved = lv
		return nil
	})
	return saved, err
}

// subject is a short label for what a record is about.
func subject(r haccp.Record) string {
	rd := r.Reading
	for _, s := range []string{rd.Unit, rd.Food, rd.Product, rd.Task} {
		if s != "" {
			if rd.Supplier != "" {
				return rd.Supplier + ": " + s
			}
			return s
		}
	}
	return ""
}
