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

func (nl NewLimit) version(id, createdBy string, at time.Time, supersedes string) haccp.LimitVersion {
	round := func(p *float64) *float64 {
		if p == nil {
			return nil
		}
		v := haccp.RoundTemp(*p)
		return &v
	}
	combos := slices.Clone(nl.Combos)
	for k := range combos {
		combos[k].MinC = haccp.RoundTemp(combos[k].MinC)
	}
	var cooling *haccp.CoolingLimit
	if nl.Cooling != nil {
		c := *nl.Cooling
		c.CutoffMaxC = haccp.RoundTemp(c.CutoffMaxC)
		cooling = &c
	}
	scope := haccp.Scope{Kind: nl.Scope.Kind, Ref: strings.TrimSpace(nl.Scope.Ref)}
	return haccp.LimitVersion{
		ID: id, Check: nl.Check, Scope: scope,
		MaxC: round(nl.MaxC), BorderlineMaxC: round(nl.BorderlineMaxC), MinC: round(nl.MinC),
		Combos: combos, Cooling: cooling, NonFood: nl.NonFood,
		Supersedes: supersedes, CreatedBy: createdBy, CreatedAt: at,
		Reason: strings.TrimSpace(nl.Reason), PlanRef: strings.TrimSpace(nl.PlanRef),
	}
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
	// BelowFSAI lists how the limit is less strict than FSAI guidance.
	BelowFSAI []string        `json:"below_fsai,omitempty"`
	Recent    []RecordRegrade `json:"recent"`
	Changed   int             `json:"changed"`
	// Problem is set when the limit cannot be saved as it is.
	Problem string `json:"problem,omitempty"`
}

// previewRecords is how many recent records a preview re-scores.
const previewRecords = 50

// PreviewLimit shows how recent records would have scored under a proposed
// limit, and anything that stops it being saved.
func (s *Service) PreviewLimit(ctx context.Context, p *auth.Principal, nl NewLimit) (LimitPreview, error) {
	if err := allow(p, auth.ActManageLimits); err != nil {
		return LimitPreview{}, err
	}
	history, err := s.st.HACCP().ListLimitVersions(ctx)
	if err != nil {
		return LimitPreview{}, err
	}
	pv := LimitPreview{Recent: []RecordRegrade{}}
	var supersedes string
	if h := head(history, nl.Check, nl.Scope); h != nil {
		pv.Replaces, supersedes = h, h.ID
	}
	lv := nl.version("(preview)", p.User.ID, s.clk.Now(), supersedes)
	pv.Limit = lv
	if err := lv.ValidateShape(); err != nil {
		pv.Problem = err.Error()
		return pv, nil
	}
	pv.BelowFSAI = lv.LooserThanFSAI()
	if err := lv.ValidateNew(); err != nil {
		pv.Problem = err.Error()
	}

	recs, err := s.st.HACCP().ListRecords(ctx, store.RecordFilter{Types: recordTypesFor(lv.Check), Newest: true, Limit: 500})
	if err != nil {
		return LimitPreview{}, err
	}
	// The proposed limit only affects records it would apply to: those whose
	// most specific matching scope, after the change, is this one.
	proposed := append(slices.Clone(history), lv)
	for _, r := range recs {
		if len(pv.Recent) == previewRecords {
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
		history, err := tx.HACCP().ListLimitVersions(ctx)
		if err != nil {
			return err
		}
		var supersedes string
		if h := head(history, nl.Check, nl.Scope); h != nil {
			supersedes = h.ID
		}
		lv := nl.version(newID(), p.User.ID, s.clk.Now(), supersedes)
		if err := lv.ValidateNew(); err != nil {
			return newErr(KindInvalid, "%s", err)
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
