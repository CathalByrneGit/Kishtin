package service

import (
	"context"
	"fmt"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// AnonymisePreview shows what anonymising a leaver would change, before
// anything changes.
type AnonymisePreview struct {
	User auth.User `json:"user"`
	// Name is what they would be called from now on, such as "Former staff 2".
	Name string `json:"name"`
	// Mentions counts the log entries that name them now.
	Mentions store.UserMentions `json:"mentions"`
	// Notes say what changes and what does not.
	Notes []string `json:"notes"`
	// Problem is set when they cannot be anonymised.
	Problem string `json:"problem,omitempty"`
}

// AnonymiseResult is what anonymising changed.
type AnonymiseResult struct {
	User    auth.User          `json:"user"`
	Changed store.UserMentions `json:"changed"`
}

// formerName picks the next free "Former staff N" name.
func formerName(users []auth.User) string {
	taken := map[string]bool{}
	n := 1
	for _, u := range users {
		taken[u.Name] = true
		if u.AnonymisedAt != nil {
			n++
		}
	}
	for {
		name := fmt.Sprintf("Former staff %d", n)
		if !taken[name] {
			return name
		}
		n++
	}
}

func (s *Service) anonymisePreview(ctx context.Context, st store.Store, id string) (AnonymisePreview, error) {
	u, err := st.Users().GetUser(ctx, id)
	if err != nil {
		return AnonymisePreview{}, mapStoreErr(err, "member of staff")
	}
	users, err := st.Users().ListUsers(ctx)
	if err != nil {
		return AnonymisePreview{}, err
	}
	m, err := st.Users().CountUserMentions(ctx, id)
	if err != nil {
		return AnonymisePreview{}, err
	}
	pv := AnonymisePreview{User: u, Name: formerName(users), Mentions: m}
	pv.Notes = []string{
		fmt.Sprintf("%s will be called %s in %d order events, %d voice commands and %d activity entries, and on the staff list.",
			u.Name, pv.Name, m.OrderEvents, m.VoiceLog, m.Activity),
		"Their PIN and password are removed and their API keys revoked.",
		fmt.Sprintf("Their %d HACCP records keep the name they were logged under: food safety records must show who did each check, and they cannot be changed.", m.HACCPRecords),
		"Names typed into records or said in voice commands, such as \"done by\" or a trainee's name, are not changed.",
		"This cannot be undone.",
	}
	switch {
	case u.AnonymisedAt != nil:
		pv.Problem = "already anonymised"
	case u.Active:
		pv.Problem = "deactivate them first: only a leaver's details are anonymised"
	}
	return pv, nil
}

// PreviewAnonymise shows what anonymising a leaver would change.
func (s *Service) PreviewAnonymise(ctx context.Context, p *auth.Principal, id string) (AnonymisePreview, error) {
	if err := s.allow(p, auth.ActManageUsers); err != nil {
		return AnonymisePreview{}, err
	}
	return s.anonymisePreview(ctx, s.st, id)
}

// Anonymise replaces a deactivated member of staff's name with "Former
// staff N" in the kitchen's logs and the staff list, removes their PIN and
// password and revokes their API keys. HACCP records keep their name.
func (s *Service) Anonymise(ctx context.Context, p *auth.Principal, id string) (AnonymiseResult, error) {
	if err := s.allow(p, auth.ActManageUsers); err != nil {
		return AnonymiseResult{}, err
	}
	var out AnonymiseResult
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		pv, err := s.anonymisePreview(ctx, tx, id)
		if err != nil {
			return err
		}
		if pv.Problem != "" {
			return newErr(KindConflict, "%s cannot be anonymised: %s", pv.User.Name, pv.Problem)
		}
		changed, err := tx.Users().AnonymiseUser(ctx, id, pv.Name)
		if err != nil {
			return err
		}
		now := s.clk.Now()
		u := pv.User
		u.Name, u.PINHash, u.PasswordHash, u.Active, u.UpdatedAt, u.AnonymisedAt = pv.Name, "", "", false, now, &now
		if err := tx.Users().UpdateUser(ctx, u); err != nil {
			return mapStoreErr(err, "a member of staff with that name")
		}
		keys, err := tx.Auth().ListAPIKeys(ctx)
		if err != nil {
			return err
		}
		revoked := 0
		for _, k := range keys {
			if k.UserID == id && k.RevokedAt == nil {
				if err := tx.Auth().RevokeAPIKey(ctx, k.ID, now); err != nil {
					return err
				}
				revoked++
			}
		}
		// The activity entry names the leaver only by their new name.
		if err := s.note(ctx, tx, p, ActivityStaffAnonymised, id, map[string]any{
			"name": pv.Name, "order_events": changed.OrderEvents, "voice_log": changed.VoiceLog,
			"activity": changed.Activity, "keys_revoked": revoked, "at": now.Format(time.RFC3339),
		}); err != nil {
			return err
		}
		out = AnonymiseResult{User: u, Changed: changed}
		return nil
	})
	if err == nil {
		s.publish(events.KindStaff, id)
	}
	return out, err
}
