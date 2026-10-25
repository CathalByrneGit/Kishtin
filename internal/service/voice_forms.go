package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
)

// The details a voice form asks for, one at a time.
const (
	slotSupplier  = "supplier"
	slotProduct   = "product"
	slotKind      = "kind"
	slotTemp      = "temp"
	slotPackaging = "packaging"
	slotInDate    = "in_date"
	slotVehicle   = "vehicle"
	slotFood      = "food"
	slotCooked    = "cooked"
	slotShelf     = "shelf"
	slotCount     = "count"
	slotReason    = "reason"
)

// missingSlot is the next detail a command needs, and the question that
// asks for it.
func missingSlot(c voice.Command) (string, string) {
	switch c.Intent {
	case voice.IntentDelivery:
		switch {
		case c.Supplier == "":
			return slotSupplier, "Who is the delivery from?"
		case c.Product == "":
			return slotProduct, "What was delivered?"
		case c.DeliveryKind == "":
			return slotKind, "Chilled, frozen or ambient?"
		case c.DeliveryKind != haccp.DeliveryAmbient && c.TempC == nil:
			return slotTemp, "What temperature is it?"
		case c.PackagingOK == nil:
			return slotPackaging, "Is the packaging intact? Yes or no."
		case c.InDate == nil:
			return slotInDate, "Is it all in date? Yes or no."
		case c.VehicleOK == nil:
			return slotVehicle, "Was the delivery vehicle clean and cold? Yes or no."
		}
	case voice.IntentCooling:
		switch {
		case c.Food == "":
			return slotFood, "What is going in to cool?"
		case c.Cooked == nil:
			return slotCooked, "When was it cooked? Say a time like 14:30, or how long ago."
		}
	case voice.IntentBatch:
		switch {
		case c.Product == "":
			return slotProduct, "What is the batch?"
		case c.ShelfDays == 0:
			return slotShelf, "How many days does it keep?"
		}
	case voice.IntentVoid:
		if c.Reason == "" {
			return slotReason, "Why is it voided?"
		}
	}
	return "", ""
}

// fillSlot fills one detail from an answer. It reports false when the
// answer could not be read.
func fillSlot(c *voice.Command, slot, text string) bool {
	switch slot {
	case slotSupplier:
		c.Supplier = voice.ReadText(text)
		return c.Supplier != ""
	case slotProduct:
		c.Product = voice.ReadText(text)
		return c.Product != ""
	case slotFood:
		c.Food = voice.ReadText(text)
		return c.Food != ""
	case slotReason:
		c.Reason = voice.ReadText(text)
		return c.Reason != ""
	case slotKind:
		k, ok := voice.ReadDeliveryKind(text)
		c.DeliveryKind = k
		return ok
	case slotTemp:
		t, ok := voice.ReadNumber(text)
		c.TempC = &t
		return ok
	case slotPackaging, slotInDate, slotVehicle:
		yes, ok := voice.ReadYesNo(text)
		switch slot {
		case slotPackaging:
			c.PackagingOK = &yes
		case slotInDate:
			c.InDate = &yes
		default:
			c.VehicleOK = &yes
		}
		return ok
	case slotCooked:
		ck, ok := voice.ReadCooked(text)
		c.Cooked = &ck
		return ok
	case slotShelf:
		d, ok := voice.ReadDays(text)
		c.ShelfDays = d
		return ok && d > 0
	case slotCount:
		if strings.Contains(strings.ToLower(text), "all") {
			c.Count = -1
			return true
		}
		n, ok := voice.ReadNumber(text)
		c.Count = int(n)
		return ok && n >= 1
	}
	return false
}

// voiceForm asks for the next missing detail, or finishes the command
// once it has them all.
func (s *Service) voiceForm(ctx context.Context, p *auth.Principal, c voice.Command) (VoiceReply, error) {
	if c.Intent == voice.IntentVoid {
		return s.voiceVoid(ctx, p, c)
	}
	if c.Intent == voice.IntentCleaning && c.Task == "" {
		return s.askTask(ctx, p, c)
	}
	if slot, ask := missingSlot(c); slot != "" {
		return s.ask(p, c, slot, ask), nil
	}
	nr, err := s.formRecord(ctx, c)
	if err != nil {
		return refused(c.Intent, err)
	}
	return s.voiceRecord(ctx, p, nr, c.Intent)
}

// ask waits for one detail.
func (s *Service) ask(p *auth.Principal, c voice.Command, slot, ask string) VoiceReply {
	pd := pending{kind: pendingForm, ask: ask, cmd: c, slot: slot, intent: c.Intent, expires: s.clk.Now().Add(pendingTTL)}
	s.voice.put(p.User.ID, pd)
	return s.waitingReply(pd, ask)
}

// askTask asks which cleaning task, with every task as an answer.
func (s *Service) askTask(ctx context.Context, p *auth.Principal, c voice.Command) (VoiceReply, error) {
	lists, err := s.Lists().current(ctx, s.st)
	if err != nil {
		return VoiceReply{}, err
	}
	var opts []voice.Option
	for _, t := range lists.Value.CleaningTasks {
		o := c
		o.Task = t.Task
		opts = append(opts, voice.Option{Label: t.Task, Command: o})
	}
	if len(opts) == 0 {
		return VoiceReply{Say: "There are no cleaning tasks on the list. A manager adds them in Setup, Fridges and tasks.", Outcome: voice.OutcomeRefused, Intent: c.Intent}, nil
	}
	pd := pending{kind: pendingChoose, ask: "Which cleaning task?", options: opts, intent: c.Intent, expires: s.clk.Now().Add(pendingTTL)}
	s.voice.put(p.User.ID, pd)
	return s.waitingReply(pd, pd.ask), nil
}

// formRecord turns a filled-in command into a HACCP record.
func (s *Service) formRecord(ctx context.Context, c voice.Command) (NewRecord, error) {
	now := s.clk.Now()
	switch c.Intent {
	case voice.IntentDelivery:
		return NewRecord{Type: haccp.RecordDelivery, Reading: haccp.Reading{
			Supplier: c.Supplier, Product: c.Product, DeliveryKind: c.DeliveryKind, TempC: c.TempC,
			PackagingOK: c.PackagingOK, InDate: c.InDate, VehicleOK: c.VehicleOK,
		}}, nil
	case voice.IntentCooling:
		cooked, err := cookedAt(*c.Cooked, now)
		if err != nil {
			return NewRecord{}, err
		}
		return NewRecord{Type: haccp.RecordCooling, Reading: haccp.Reading{Food: c.Food, CookedAt: &cooked, IntoFridgeAt: &now}}, nil
	case voice.IntentBatch:
		days := c.ShelfDays
		return NewRecord{Type: haccp.RecordBatchLabel, Reading: haccp.Reading{
			Product: c.Product, MadeOn: now.In(clock.Dublin).Format(haccp.DateLayout), ShelfLifeDays: &days, Quantity: c.Quantity,
		}}, nil
	case voice.IntentCleaning:
		yes := true
		return NewRecord{Type: haccp.RecordCleaning, Reading: haccp.Reading{Task: c.Task, Done: &yes}}, nil
	}
	return NewRecord{}, fmt.Errorf("no record for %s", c.Intent)
}

// cookedAt turns "14:30" (today, Dublin) or "90 minutes ago" into a time.
// A clock time later than now means yesterday, such as a stock cooked
// before midnight.
func cookedAt(c voice.Cooked, now time.Time) (time.Time, error) {
	if c.Clock == "" {
		return now.Add(-c.Ago), nil
	}
	t, err := time.ParseInLocation("15:04", c.Clock, clock.Dublin)
	if err != nil {
		return time.Time{}, newErr(KindInvalid, "could not read the time %q", c.Clock)
	}
	d := now.In(clock.Dublin)
	at := time.Date(d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), 0, 0, clock.Dublin)
	if at.After(now.Add(time.Minute)) {
		at = at.AddDate(0, 0, -1)
	}
	return at, nil
}

// voiceVoid works out which items to void, then asks why, then asks for
// "confirm void". Nothing is voided until then.
func (s *Service) voiceVoid(ctx context.Context, p *auth.Principal, c voice.Command) (VoiceReply, error) {
	if err := allow(p, auth.ActRunPass); err != nil {
		return refused(c.Intent, err)
	}
	if len(c.ItemIDs) == 0 {
		open, err := s.st.Orders().ListOrders(ctx, store.OrderFilter{OpenOnly: true})
		if err != nil {
			return VoiceReply{}, err
		}
		var cands []orders.Item
		for _, o := range open {
			if !slices.Contains(c.OrderIDs, o.ID) {
				continue
			}
			for _, it := range o.Items {
				if it.DishID == c.DishID && !it.State.Done() {
					cands = append(cands, it)
				}
			}
		}
		if len(cands) == 0 {
			return VoiceReply{Say: fmt.Sprintf("No %s to void for %s.", s.dishSaid(ctx, c.DishID), s.labels(ctx, c.OrderIDs)), Outcome: voice.OutcomeRefused, Intent: c.Intent}, nil
		}
		n := c.Count
		switch {
		case n == 0 && len(cands) == 1:
			n = 1
		case n == 0:
			return s.ask(p, c, slotCount, fmt.Sprintf("How many: there are %d %s for %s. Say a number, or all.", len(cands), cands[0].DishName, s.labels(ctx, c.OrderIDs))), nil
		case n < 0 || n > len(cands):
			n = len(cands)
		}
		// Items in different states: ask which, rather than guess.
		byState := map[orders.State][]orders.Item{}
		var states []orders.State
		for _, it := range cands {
			if _, ok := byState[it.State]; !ok {
				states = append(states, it.State)
			}
			byState[it.State] = append(byState[it.State], it)
		}
		if n < len(cands) && len(states) > 1 {
			var opts []voice.Option
			for _, st := range states {
				group := byState[st]
				if len(group) < n {
					continue
				}
				o := c
				o.ItemIDs = itemIDs(group[:n])
				opts = append(opts, voice.Option{Label: fmt.Sprintf("%d %s, %s", n, group[0].DishName, itemStateWords(st)), Command: o})
			}
			if len(opts) > 1 {
				labels := make([]string, len(opts))
				for i, o := range opts {
					labels[i] = o.Label
				}
				pd := pending{kind: pendingChoose, ask: "Which: " + orList(labels) + "?", options: opts, intent: c.Intent, expires: s.clk.Now().Add(pendingTTL)}
				s.voice.put(p.User.ID, pd)
				return s.waitingReply(pd, pd.ask), nil
			}
			if len(opts) == 1 {
				c.ItemIDs = opts[0].Command.ItemIDs
			}
		}
		if len(c.ItemIDs) == 0 {
			// Most advanced first: a dropped plate is the one already cooking.
			slices.SortStableFunc(cands, func(a, b orders.Item) int {
				return slices.Index(orders.States, b.State) - slices.Index(orders.States, a.State)
			})
			c.ItemIDs = itemIDs(cands[:n])
		}
	}
	if slot, ask := missingSlot(c); slot != "" {
		return s.ask(p, c, slot, ask), nil
	}
	what, err := s.itemsSaid(ctx, c.ItemIDs)
	if err != nil {
		return VoiceReply{}, err
	}
	ask := fmt.Sprintf("Void %s: %s. Say confirm void, or cancel.", what, c.Reason)
	pd := pending{kind: pendingVoid, ask: ask, cmd: c, intent: c.Intent, expires: s.clk.Now().Add(pendingTTL)}
	s.voice.put(p.User.ID, pd)
	return s.waitingReply(pd, ask), nil
}

// voidItems voids the confirmed items with their reason.
func (s *Service) voidItems(ctx context.Context, p *auth.Principal, c voice.Command) (VoiceReply, error) {
	what, err := s.itemsSaid(ctx, c.ItemIDs)
	if err != nil {
		return VoiceReply{}, err
	}
	out, err := s.MoveItems(ctx, p, ItemMove{ItemIDs: c.ItemIDs, To: orders.StateVoided, Reason: c.Reason + " (by voice)"})
	if err != nil {
		return refused(c.Intent, err)
	}
	ref := ""
	if len(out) > 0 {
		ref = out[0].ID
	}
	return done(c.Intent, ref, "Voided %s.", what)
}

func itemIDs(items []orders.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

// itemsSaid names items: "2 Burger (cooking) for Table 4".
func (s *Service) itemsSaid(ctx context.Context, ids []string) (string, error) {
	open, err := s.st.Orders().ListOrders(ctx, store.OrderFilter{OpenOnly: true})
	if err != nil {
		return "", err
	}
	type key struct{ dish, state, label string }
	counts := map[key]int{}
	var keys []key
	for _, o := range open {
		for _, it := range o.Items {
			if slices.Contains(ids, it.ID) {
				k := key{itemWords(it), itemStateWords(it.State), o.Label()}
				if counts[k] == 0 {
					keys = append(keys, k)
				}
				counts[k]++
			}
		}
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%d %s (%s) for %s", counts[k], k.dish, k.state, k.label)
	}
	return joinAnd(parts), nil
}

func (s *Service) dishSaid(ctx context.Context, id string) string {
	if d, err := s.st.Menu().GetDish(ctx, id); err == nil {
		return d.Name
	}
	return "such dish"
}

func (s *Service) labels(ctx context.Context, ids []string) string {
	var out []string
	for _, id := range ids {
		if o, err := s.st.Orders().GetOrder(ctx, id); err == nil {
			out = append(out, o.Label())
		}
	}
	return joinAnd(out)
}
