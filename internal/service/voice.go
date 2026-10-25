package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/scheduler"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
)

// pendingTTL is how long a question waits for its answer.
const pendingTTL = 60 * time.Second

// maxAudioBytes caps one push-to-talk recording (about a minute of Opus).
const maxAudioBytes = 4 << 20

// VoiceReply is what Kishtin says back to a voice command.
type VoiceReply struct {
	// Heard is the transcript, as heard.
	Heard string `json:"heard"`
	// Say is the reply, to show and to speak.
	Say     string        `json:"say"`
	Outcome voice.Outcome `json:"outcome"`
	Intent  voice.Intent  `json:"intent,omitempty"`
	// Options are the answers to a question, for buttons on screen.
	Options []string `json:"options,omitempty"`
	// Confirm is set when the reply waits for "confirm" or "cancel".
	Confirm bool `json:"confirm,omitempty"`
	// Waiting is set when the next command answers this reply: a question,
	// a corrective action, or a confirmation. It expires after a minute.
	Waiting bool `json:"waiting,omitempty"`
	// Ref is the order, record or dish acted on.
	Ref string `json:"ref,omitempty"`
}

// VoiceStatus says which speech sidecars are set up.
type VoiceStatus struct {
	// Listen is set when speech-to-text is set up, so spoken commands work.
	Listen bool `json:"listen"`
	// Speak is set when text-to-speech is set up, so replies are spoken.
	Speak bool `json:"speak"`
	// Waiting is the question waiting for this person's answer, if any.
	Waiting string `json:"waiting,omitempty"`
}

type pendingKind int

const (
	pendingChoose  pendingKind = iota + 1 // pick one of options
	pendingAction                         // say the corrective action
	pendingConfirm                        // say confirm or cancel
)

type pending struct {
	kind    pendingKind
	ask     string
	options []voice.Option
	record  NewRecord
	intent  voice.Intent
	expires time.Time
}

// voiceState holds each person's open question.
type voiceState struct {
	mu      sync.Mutex
	pending map[string]pending
}

func (v *voiceState) take(user string, now time.Time) (pending, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.pending[user]
	delete(v.pending, user)
	return p, ok && now.Before(p.expires)
}

func (v *voiceState) put(user string, p pending) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.pending == nil {
		v.pending = map[string]pending{}
	}
	v.pending[user] = p
}

func (v *voiceState) peek(user string, now time.Time) (pending, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.pending[user]
	return p, ok && now.Before(p.expires)
}

// VoiceStatus says which speech sidecars are set up, and what is waiting
// for an answer.
func (s *Service) VoiceStatus(_ context.Context, p *auth.Principal) (VoiceStatus, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return VoiceStatus{}, err
	}
	st := VoiceStatus{Listen: s.speech.CanListen(), Speak: s.speech.CanSpeak()}
	if pd, ok := s.voice.peek(p.User.ID, s.clk.Now()); ok {
		st.Waiting = pd.ask
	}
	return st, nil
}

// VoiceAudio transcribes push-to-talk audio with the local speech-to-text
// sidecar, then runs the command. The audio is not stored.
func (s *Service) VoiceAudio(ctx context.Context, p *auth.Principal, audio []byte, contentType string) (VoiceReply, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return VoiceReply{}, err
	}
	if !s.speech.CanListen() {
		return VoiceReply{}, newErr(KindConflict, "Speech-to-text is not set up on this server, so spoken commands cannot be understood. Type the command instead, or ask a manager to set speech.stt_url in kishtin.toml.")
	}
	if len(audio) == 0 {
		return VoiceReply{}, newErr(KindInvalid, "No audio was recorded. Hold the button while you speak.")
	}
	if len(audio) > maxAudioBytes {
		return VoiceReply{}, newErr(KindInvalid, "The recording is too long. Keep commands short, under a minute.")
	}
	v, err := s.vocab(ctx)
	if err != nil {
		return VoiceReply{}, err
	}
	text, err := s.speech.Transcribe(ctx, audio, contentType, prompt(v))
	if err != nil {
		return VoiceReply{}, newErr(KindConflict, "Could not reach speech-to-text: %s. Type the command instead.", err)
	}
	return s.voiceCommand(ctx, p, text, voice.SourceSpoken, v)
}

// VoiceSpeak turns a reply into speech with the local text-to-speech
// sidecar and returns WAV audio.
func (s *Service) VoiceSpeak(ctx context.Context, p *auth.Principal, text string) ([]byte, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > 500 {
		return nil, newErr(KindInvalid, "text must be 1 to 500 characters")
	}
	if !s.speech.CanSpeak() {
		return nil, newErr(KindConflict, "Text-to-speech is not set up on this server. Set speech.tts_url in kishtin.toml.")
	}
	audio, err := s.speech.Speak(ctx, text)
	if err != nil {
		return nil, newErr(KindConflict, "Could not reach text-to-speech: %s", err)
	}
	return audio, nil
}

// VoiceCommand runs a typed command through the same grammar as speech.
func (s *Service) VoiceCommand(ctx context.Context, p *auth.Principal, text string) (VoiceReply, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return VoiceReply{}, err
	}
	if len([]rune(text)) > 300 {
		return VoiceReply{}, newErr(KindInvalid, "a command is at most 300 characters")
	}
	v, err := s.vocab(ctx)
	if err != nil {
		return VoiceReply{}, err
	}
	return s.voiceCommand(ctx, p, text, voice.SourceTyped, v)
}

// VoiceLog lists recent voice commands, newest first.
func (s *Service) VoiceLog(ctx context.Context, p *auth.Principal, limit int) ([]voice.LogEntry, error) {
	if err := allow(p, auth.ActViewKitchen); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.st.Voice().ListVoiceLog(ctx, time.Time{}, limit)
}

// vocab is what can be named right now: the menu, stations, fridges and
// open orders.
func (s *Service) vocab(ctx context.Context) (voice.Vocab, error) {
	var v voice.Vocab
	setup, err := menuSetup(ctx, s.st)
	if err != nil {
		return v, err
	}
	for _, d := range setup.Dishes {
		if d.Active {
			v.Dishes = append(v.Dishes, voice.Name{ID: d.ID, Name: d.Name})
		}
	}
	for _, st := range setup.Stations {
		if st.Active {
			v.Stations = append(v.Stations, voice.Name{ID: st.ID, Name: st.Name})
		}
	}
	lists, err := s.Lists().current(ctx, s.st)
	if err != nil {
		return v, err
	}
	for _, u := range lists.Value.Units {
		v.Units = append(v.Units, voice.Unit{Name: u.Name, Kind: string(u.Kind)})
	}
	open, err := s.st.Orders().ListOrders(ctx, store.OrderFilter{OpenOnly: true})
	if err != nil {
		return v, err
	}
	for _, o := range open {
		v.Orders = append(v.Orders, voice.OrderRef{ID: o.ID, Table: o.Table, Number: o.Number, Label: o.Label()})
	}
	return v, nil
}

// prompt biases speech recognition toward what can be said right now.
func prompt(v voice.Vocab) string {
	var b strings.Builder
	b.WriteString("Kitchen commands: fire, hold, ready, 86, back on, what's next on, core temp, reheat, hot hold, degrees, confirm, cancel.")
	add := func(label string, names []string) {
		if len(names) > 0 {
			b.WriteString(" " + label + ": " + strings.Join(names, ", ") + ".")
		}
	}
	var tables, dishes, stations, units []string
	for _, o := range v.Orders {
		tables = append(tables, o.Label)
	}
	for _, d := range v.Dishes {
		dishes = append(dishes, d.Name)
	}
	for _, st := range v.Stations {
		stations = append(stations, st.Name)
	}
	for _, u := range v.Units {
		units = append(units, u.Name)
	}
	add("Open", tables)
	add("Stations", stations)
	add("Fridges", units)
	add("Dishes", dishes)
	out := b.String()
	if r := []rune(out); len(r) > 900 {
		out = string(r[:900])
	}
	return out
}

func (s *Service) voiceCommand(ctx context.Context, p *auth.Principal, text string, src voice.Source, v voice.Vocab) (VoiceReply, error) {
	text = strings.TrimSpace(text)
	reply, err := s.answer(ctx, p, text, v)
	if err != nil {
		return VoiceReply{}, err
	}
	reply.Heard = text
	entry := voice.LogEntry{
		ID: newID(), At: s.clk.Now(), UserID: p.User.ID, UserName: p.User.Name, Source: src,
		Transcript: text, Intent: reply.Intent, Outcome: reply.Outcome, Reply: reply.Say, Ref: reply.Ref,
	}
	if err := s.st.Voice().AppendVoiceLog(ctx, entry); err != nil {
		return VoiceReply{}, fmt.Errorf("log voice command: %w", err)
	}
	return reply, nil
}

// answer works out the reply, using and updating the person's open
// question.
func (s *Service) answer(ctx context.Context, p *auth.Principal, text string, v voice.Vocab) (VoiceReply, error) {
	now := s.clk.Now()
	r := voice.Parse(text, v)
	isConfirm := r.Command != nil && r.Command.Intent == voice.IntentConfirm
	isCancel := r.Command != nil && r.Command.Intent == voice.IntentCancel

	if pd, ok := s.voice.take(p.User.ID, now); ok {
		if isCancel {
			return VoiceReply{Say: "Cancelled. Nothing was saved.", Outcome: voice.OutcomeCancelled, Intent: pd.intent}, nil
		}
		keep := func(say string) (VoiceReply, error) {
			pd.expires = now.Add(pendingTTL)
			s.voice.put(p.User.ID, pd)
			return s.waitingReply(pd, say), nil
		}
		switch pd.kind {
		case pendingChoose:
			if i, ok := voice.Choose(text, pd.options); ok {
				return s.execute(ctx, p, pd.options[i].Command)
			}
			// A new, complete command replaces the question.
			if r.Command != nil && !isConfirm {
				return s.execute(ctx, p, *r.Command)
			}
			return keep("Sorry, I did not catch that. " + pd.ask)
		case pendingAction:
			if isConfirm {
				return keep("Say what you did about it first, or say cancel.")
			}
			pd.record.CorrectiveAction = sentence(text)
			pd.kind = pendingConfirm
			pd.ask = fmt.Sprintf("Action: %s. Say confirm to save, or cancel.", pd.record.CorrectiveAction)
			return keep(pd.ask)
		case pendingConfirm:
			if !isConfirm {
				return keep("Say confirm to save, or cancel.")
			}
			return s.saveRecord(ctx, p, pd.record, pd.intent)
		}
	}

	switch {
	case r.Problem != "":
		return VoiceReply{Say: r.Problem, Outcome: voice.OutcomeNotUnderstood}, nil
	case r.Ask != "":
		pd := pending{kind: pendingChoose, ask: r.Ask, options: r.Options, expires: now.Add(pendingTTL)}
		if len(r.Options) > 0 {
			pd.intent = r.Options[0].Command.Intent
		}
		s.voice.put(p.User.ID, pd)
		return s.waitingReply(pd, r.Ask), nil
	case isConfirm || isCancel:
		return VoiceReply{Say: "Nothing is waiting for an answer.", Outcome: voice.OutcomeNotUnderstood, Intent: r.Command.Intent}, nil
	}
	return s.execute(ctx, p, *r.Command)
}

func (s *Service) waitingReply(pd pending, say string) VoiceReply {
	r := VoiceReply{Say: say, Outcome: voice.OutcomeAsked, Intent: pd.intent, Waiting: true, Confirm: pd.kind == pendingConfirm}
	for _, o := range pd.options {
		r.Options = append(r.Options, o.Label)
	}
	return r
}

// sentence capitalises the first letter of what was said.
func sentence(s string) string {
	s = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), "."))
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

// refused turns a service error into a spoken refusal. Other errors are
// returned as they are.
func refused(i voice.Intent, err error) (VoiceReply, error) {
	var e *Error
	if errors.As(err, &e) {
		return VoiceReply{Say: e.Msg, Outcome: voice.OutcomeRefused, Intent: i}, nil
	}
	return VoiceReply{}, err
}

func done(i voice.Intent, ref, format string, args ...any) (VoiceReply, error) {
	return VoiceReply{Say: fmt.Sprintf(format, args...), Outcome: voice.OutcomeDone, Intent: i, Ref: ref}, nil
}

// execute carries out an understood command through the same service
// functions the screens use, with the same permissions.
func (s *Service) execute(ctx context.Context, p *auth.Principal, c voice.Command) (VoiceReply, error) {
	switch c.Intent {
	case voice.IntentFire, voice.IntentHold, voice.IntentReady:
		return s.voiceCourse(ctx, p, c)
	case voice.IntentNext:
		return s.voiceNext(ctx, p, c)
	case voice.IntentStart:
		return s.voiceStart(ctx, p, c)
	case voice.IntentSoldOut, voice.IntentBackOn:
		d, err := s.SetAvailability(ctx, p, c.DishID, c.Intent == voice.IntentBackOn)
		if err != nil {
			return refused(c.Intent, err)
		}
		if c.Intent == voice.IntentBackOn {
			return done(c.Intent, d.ID, "%s is back on.", d.Name)
		}
		return done(c.Intent, d.ID, "%s is 86.", d.Name)
	case voice.IntentUnitTemp, voice.IntentFoodTemp:
		return s.voiceTemp(ctx, p, c)
	}
	return VoiceReply{Say: "Nothing is waiting for an answer.", Outcome: voice.OutcomeNotUnderstood, Intent: c.Intent}, nil
}

var courseSaid = map[menu.Course]string{menu.CourseStarter: "starters", menu.CourseMain: "mains", menu.CourseDessert: "desserts"}

// voiceCourse fires, holds or readies a course. With no course said, it
// picks the obvious one: the next held course to fire, or the fired
// course to hold or ready.
func (s *Service) voiceCourse(ctx context.Context, p *auth.Principal, c voice.Command) (VoiceReply, error) {
	var says []string
	var ref string
	for _, id := range c.OrderIDs {
		o, err := s.st.Orders().GetOrder(ctx, id)
		if err != nil {
			return VoiceReply{}, err
		}
		course := menu.Course(c.Course)
		if course == "" {
			want := orders.CourseFired
			if c.Intent == voice.IntentFire {
				want = orders.CourseHeld
			}
			var best *orders.Course
			for i := range o.Courses {
				oc := &o.Courses[i]
				if oc.State != want {
					continue
				}
				if best == nil || (want == orders.CourseFired && oc.TargetAt != nil && best.TargetAt != nil && oc.TargetAt.Before(*best.TargetAt)) {
					best = oc
				}
			}
			if best == nil {
				what := "a held course to fire"
				if want == orders.CourseFired {
					what = "a fired course"
				}
				return VoiceReply{Say: fmt.Sprintf("%s has no %s.", o.Label(), what), Outcome: voice.OutcomeRefused, Intent: c.Intent}, nil
			}
			course = best.Course
		}
		var out orders.Order
		switch c.Intent {
		case voice.IntentFire:
			out, err = s.FireCourse(ctx, p, id, course)
		case voice.IntentHold:
			out, err = s.HoldCourse(ctx, p, id, course, "by voice")
		default:
			out, err = s.ReadyCourse(ctx, p, id, course)
		}
		if err != nil {
			return refused(c.Intent, err)
		}
		ref = out.ID
		switch c.Intent {
		case voice.IntentFire:
			oc, _ := out.Course(course)
			due := ""
			if oc.TargetAt != nil {
				due = ", due " + oc.TargetAt.In(clock.Dublin).Format("15:04")
			}
			says = append(says, fmt.Sprintf("%s %s fired%s.", out.Label(), courseSaid[course], due))
		case voice.IntentHold:
			says = append(says, fmt.Sprintf("%s %s held again.", out.Label(), courseSaid[course]))
		default:
			says = append(says, fmt.Sprintf("%s %s ready at the pass.", out.Label(), courseSaid[course]))
		}
	}
	return done(c.Intent, ref, "%s", strings.Join(says, " "))
}

// stationPlan returns the plan for one station.
func (s *Service) stationPlan(ctx context.Context, p *auth.Principal, stationID string) (KitchenView, scheduler.StationPlan, error) {
	view, err := s.Kitchen(ctx, p)
	if err != nil {
		return view, scheduler.StationPlan{}, err
	}
	for _, sp := range view.Plan.Stations {
		if sp.StationID == stationID {
			return view, sp, nil
		}
	}
	return view, scheduler.StationPlan{}, nil
}

// spokenAction says an action the way a chef would: "3 Burgers (Well done)
// for Table 4 and Table 7".
func spokenAction(a scheduler.Action) string {
	name := a.DishName
	if len(a.Modifiers) > 0 {
		name += " (" + strings.Join(a.Modifiers, ", ") + ")"
	}
	return fmt.Sprintf("%d %s for %s", a.Quantity, name, joinAnd(a.Labels))
}

func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// voiceNext reads a station's next recommendations, with their reasons
// shown on screen.
func (s *Service) voiceNext(ctx context.Context, p *auth.Principal, c voice.Command) (VoiceReply, error) {
	_, sp, err := s.stationPlan(ctx, p, c.StationID)
	if err != nil {
		return refused(c.Intent, err)
	}
	name := sp.Name
	if name == "" {
		name = "That station"
	}
	if len(sp.Actions) == 0 {
		return VoiceReply{Say: fmt.Sprintf("Nothing to start on %s.", name), Outcome: voice.OutcomeAnswered, Intent: c.Intent, Ref: c.StationID}, nil
	}
	var parts []string
	for i, a := range sp.Actions {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("and %d more", len(sp.Actions)-3))
			break
		}
		when := "Start now: "
		if !a.Now {
			when = "At " + a.StartAt.In(clock.Dublin).Format("15:04") + ": "
		}
		parts = append(parts, when+spokenAction(a)+".")
	}
	return VoiceReply{Say: name + ". " + strings.Join(parts, " "), Outcome: voice.OutcomeAnswered, Intent: c.Intent, Ref: c.StationID}, nil
}

// voiceStart marks a dish cooking for the tables named. With no table
// said, it starts what the scheduler says to start now.
func (s *Service) voiceStart(ctx context.Context, p *auth.Principal, c voice.Command) (VoiceReply, error) {
	view, err := s.Kitchen(ctx, p)
	if err != nil {
		return refused(c.Intent, err)
	}
	dishName := ""
	var ids, labels []string
	add := func(o orders.Order, it orders.Item) {
		if c.Count > 0 && len(ids) >= c.Count {
			return
		}
		ids = append(ids, it.ID)
		dishName = it.DishName
		if !slices.Contains(labels, o.Label()) {
			labels = append(labels, o.Label())
		}
	}
	if len(c.OrderIDs) > 0 {
		for _, id := range c.OrderIDs {
			for _, o := range view.Orders {
				if o.ID != id {
					continue
				}
				for _, it := range o.Items {
					if it.DishID == c.DishID && it.State == orders.StateFired {
						add(o, it)
					}
				}
			}
		}
	} else {
		// The scheduler's "start now" for this dish.
		for _, sp := range view.Plan.Stations {
			for _, a := range sp.Actions {
				if a.DishID != c.DishID || !a.Now || len(ids) > 0 {
					continue
				}
				for _, itemID := range a.ItemIDs {
					for _, o := range view.Orders {
						if it, ok := o.Item(itemID); ok {
							add(o, it)
						}
					}
				}
			}
		}
		if len(ids) == 0 {
			// Nothing to start now: if it waits for one order only, that
			// is clear; for several, ask.
			var opts []voice.Option
			for _, o := range view.Orders {
				for _, it := range o.Items {
					if it.DishID == c.DishID && it.State == orders.StateFired {
						opts = append(opts, voice.Option{Label: o.Label(), Command: voice.Command{Intent: voice.IntentStart, DishID: c.DishID, OrderIDs: []string{o.ID}, Count: c.Count, Words: c.Words}})
						break
					}
				}
			}
			switch len(opts) {
			case 0:
			case 1:
				return s.execute(ctx, p, opts[0].Command)
			default:
				labels := make([]string, len(opts))
				for i, o := range opts {
					labels[i] = o.Label
				}
				pd := pending{kind: pendingChoose, ask: "For " + orList(labels) + "?", options: opts, intent: voice.IntentStart, expires: s.clk.Now().Add(pendingTTL)}
				s.voice.put(p.User.ID, pd)
				return s.waitingReply(pd, pd.ask), nil
			}
		}
	}
	if len(ids) == 0 {
		name := c.DishID
		if d, err := s.st.Menu().GetDish(ctx, c.DishID); err == nil {
			name = d.Name
		}
		where := ""
		if len(c.OrderIDs) > 0 {
			var ls []string
			for _, id := range c.OrderIDs {
				for _, o := range view.Orders {
					if o.ID == id {
						ls = append(ls, o.Label())
					}
				}
			}
			where = " for " + joinAnd(ls)
		}
		return VoiceReply{Say: fmt.Sprintf("No %s waiting to start%s.", name, where), Outcome: voice.OutcomeRefused, Intent: c.Intent}, nil
	}
	if _, err := s.MoveItems(ctx, p, ItemMove{ItemIDs: ids, To: orders.StateCooking, From: orders.StateFired}); err != nil {
		return refused(c.Intent, err)
	}
	return done(c.Intent, c.DishID, "%d %s on for %s.", len(ids), dishName, joinAnd(labels))
}

func orList(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

// voiceTemp logs a temperature check. A pass is saved straight away. A
// reading that needs a corrective action asks for it, then asks for a
// spoken confirmation before saving.
func (s *Service) voiceTemp(ctx context.Context, p *auth.Principal, c voice.Command) (VoiceReply, error) {
	if c.TempC == nil {
		return VoiceReply{Say: "No temperature was heard.", Outcome: voice.OutcomeNotUnderstood, Intent: c.Intent}, nil
	}
	nr := NewRecord{Reading: haccp.Reading{TempC: c.TempC}}
	what := ""
	if c.Intent == voice.IntentUnitTemp {
		nr.Type = haccp.RecordType(c.UnitKind)
		nr.Reading.Unit = c.Unit
		what = c.Unit
	} else {
		nr.Type = haccp.RecordType(c.Check)
		nr.Reading.Food = sentence(c.Food)
		if c.DishID != "" {
			if d, err := s.st.Menu().GetDish(ctx, c.DishID); err == nil {
				nr.Reading.Food, nr.Reading.Dish, nr.Reading.FoodCategory = d.Name, d.Name, d.FoodCategory
			}
		}
		what = nr.Reading.Food
	}
	check, err := s.CheckRecord(ctx, p, nr)
	if err != nil {
		return refused(c.Intent, err)
	}
	reading := fmt.Sprintf("%s %s degrees", what, formatTemp(*c.TempC))
	if !check.NeedsCorrectiveAction {
		return s.saveRecord(ctx, p, nr, c.Intent)
	}
	ask := fmt.Sprintf("%s: %s. %s What did you do about it? Or say cancel.", reading, outcomeWord(check.Result.Outcome), check.Result.Reason)
	pd := pending{kind: pendingAction, ask: ask, record: nr, intent: c.Intent, expires: s.clk.Now().Add(pendingTTL)}
	s.voice.put(p.User.ID, pd)
	return s.waitingReply(pd, ask), nil
}

func (s *Service) saveRecord(ctx context.Context, p *auth.Principal, nr NewRecord, i voice.Intent) (VoiceReply, error) {
	rec, err := s.LogRecord(ctx, p, nr)
	if err != nil {
		return refused(i, err)
	}
	what := rec.Reading.Unit
	if what == "" {
		what = rec.Reading.Food
	}
	say := fmt.Sprintf("%s %s degrees. %s. Saved.", what, formatTemp(*rec.Reading.TempC), outcomeWord(rec.Outcome))
	if rec.Outcome != haccp.OutcomePass && rec.OutcomeReason != "" {
		say = fmt.Sprintf("%s %s degrees. %s: %s Saved.", what, formatTemp(*rec.Reading.TempC), outcomeWord(rec.Outcome), rec.OutcomeReason)
	}
	return done(i, rec.ID, "%s", say)
}

func outcomeWord(o haccp.Outcome) string {
	switch o {
	case haccp.OutcomePass:
		return "Pass"
	case haccp.OutcomeBorderline:
		return "Borderline"
	case haccp.OutcomeFail:
		return "Fail"
	}
	return "Recorded"
}

func formatTemp(t float64) string {
	return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", t), "0"), ".")
}
