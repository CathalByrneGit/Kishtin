// Package voice understands spoken kitchen commands. It matches a
// transcript against a fixed grammar, with fuzzy matching on dish names,
// tables, stations and fridges. It does no I/O: the caller passes in what
// is on the menu and which orders are open, and gets back a command, a
// question to ask, or what was not understood.
package voice

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Intent is what a command does.
type Intent string

const (
	// IntentFire fires a course: "fire table 4 mains".
	IntentFire Intent = "fire"
	// IntentHold holds a fired course again: "hold table 4 mains".
	IntentHold Intent = "hold"
	// IntentNext reads a station's next recommendations: "what's next on grill".
	IntentNext Intent = "next"
	// IntentStart marks items cooking: "burgers on, tables 4 and 7".
	IntentStart Intent = "start"
	// IntentReady marks a course ready at the pass: "table 7 ready".
	IntentReady Intent = "ready"
	// IntentSoldOut marks a dish sold out: "86 salmon".
	IntentSoldOut Intent = "sold_out"
	// IntentBackOn puts a sold-out dish back on: "salmon back on".
	IntentBackOn Intent = "back_on"
	// IntentUnitTemp logs a fridge or freezer reading: "walk-in fridge 3.4 degrees".
	IntentUnitTemp Intent = "unit_temp"
	// IntentFoodTemp logs a cooking, reheating or hot-holding check: "core temp chicken 78".
	IntentFoodTemp Intent = "food_temp"
	// IntentConfirm and IntentCancel answer a question.
	IntentConfirm Intent = "confirm"
	IntentCancel  Intent = "cancel"
)

// Name is something on the menu or in the kitchen that can be said.
type Name struct {
	ID   string
	Name string
}

// Unit is a fridge or freezer.
type Unit struct {
	Name string
	// Kind is "fridge" or "freezer".
	Kind string
}

// OrderRef is an open order that can be named by table or ticket number.
type OrderRef struct {
	ID string
	// Table is the table name, empty for a takeaway.
	Table  string
	Number int
	Label  string
}

// Vocab is what can be named right now.
type Vocab struct {
	Dishes   []Name
	Stations []Name
	Units    []Unit
	Orders   []OrderRef
}

// Command is a fully understood command.
type Command struct {
	Intent Intent `json:"intent"`
	// OrderIDs are the orders named, in the order said.
	OrderIDs []string `json:"order_ids,omitempty"`
	// Course is starter, main or dessert; empty when not said.
	Course    string `json:"course,omitempty"`
	DishID    string `json:"dish_id,omitempty"`
	StationID string `json:"station_id,omitempty"`
	// Count is how many were said ("two burgers on"); 0 when not said.
	Count int `json:"count,omitempty"`
	// Unit and UnitKind name the fridge or freezer for a unit reading.
	Unit     string `json:"unit,omitempty"`
	UnitKind string `json:"unit_kind,omitempty"`
	// Check is cook, reheat or hot_hold for a food reading.
	Check string `json:"check,omitempty"`
	// Food is what was checked, as said, for a food reading.
	Food  string   `json:"food,omitempty"`
	TempC *float64 `json:"temp_c,omitempty"`
	// Words is what was said, normalized, for logging.
	Words string `json:"words"`
}

// Option is one answer to a question, with the command it stands for.
type Option struct {
	Label   string  `json:"label"`
	Command Command `json:"command"`
}

// Result is what a transcript means.
type Result struct {
	// Command is set when the transcript was understood.
	Command *Command
	// Ask is set when it could mean more than one thing: "Table 4 or
	// table 14?". Never guess.
	Ask     string
	Options []Option
	// Problem says what was not understood.
	Problem string
}

// Parse matches a transcript against the grammar.
func Parse(transcript string, v Vocab) Result {
	w := normalize(transcript)
	words := strings.Join(w, " ")
	if len(w) == 0 {
		return Result{Problem: "Nothing was heard."}
	}
	p := parser{w: w, v: v, words: words}
	return p.parse()
}

type parser struct {
	w     []string
	v     Vocab
	words string
}

var (
	confirmWords = []string{"confirm", "confirmed", "confirm void", "confirm fail", "yes", "yes confirm", "confirm it", "do it", "yeah", "yep", "go ahead"}
	cancelWords  = []string{"cancel", "no", "stop", "never mind", "nevermind", "forget it", "cancel that", "scratch that"}
	// fireWords are how "fire" comes out of a speech model in a loud
	// kitchen, as the first word of a command.
	fireWords = []string{"fire", "fired", "fryer", "fires", "fyre", "far", "fair"}
)

func (p *parser) cmd(i Intent) *Command { return &Command{Intent: i, Words: p.words} }

func (p *parser) parse() Result {
	if slices.Contains(confirmWords, p.words) {
		return Result{Command: p.cmd(IntentConfirm)}
	}
	if slices.Contains(cancelWords, p.words) {
		return Result{Command: p.cmd(IntentCancel)}
	}
	first := p.w[0]
	switch {
	case slices.Contains(fireWords, first) && p.hasOrderRef():
		return p.course(IntentFire, 1)
	case first == "hold" && p.hasOrderRef():
		return p.course(IntentHold, 1)
	case first == "86" || (first == "eighty" && len(p.w) > 1 && p.w[1] == "six"):
		return p.dishOnly(IntentSoldOut)
	case p.has("next") || strings.HasPrefix(p.words, "whats on ") || strings.HasPrefix(p.words, "what is on "):
		return p.next()
	case p.hasPhrase("back on") || first == "un86" || (first == "un" && len(p.w) > 1 && p.w[1] == "86"):
		return p.dishOnly(IntentBackOn)
	case p.has("core") || p.has("probe") || p.hasPhrase("cook check") || p.has("reheat") || p.has("reheated") || p.hasPhrase("hot hold") || p.has("holding"):
		return p.foodTemp()
	}
	if r, ok := p.unitTemp(); ok {
		return r
	}
	if p.hasOrderRef() && (p.has("ready") || p.has("up")) && !p.has("on") {
		return p.course(IntentReady, 0)
	}
	if p.has("away") && p.hasOrderRef() {
		return p.course(IntentFire, 0)
	}
	if p.has("on") || p.has("start") || p.has("started") || p.has("going") || p.has("down") {
		return p.start()
	}
	return Result{Problem: fmt.Sprintf("Heard %q, but that is not a command I know. Try \"fire table 4 mains\" or \"walk-in fridge 3 degrees\".", p.words)}
}

func (p *parser) has(word string) bool { return slices.Contains(p.w, word) }

func (p *parser) hasPhrase(s string) bool {
	return strings.Contains(" "+p.words+" ", " "+s+" ")
}

// --- orders ---

var orderWords = []string{"table", "tables", "takeaway", "takeaways", "ticket", "number"}

func (p *parser) hasOrderRef() bool {
	for _, w := range p.w {
		if slices.Contains(orderWords, w) {
			return true
		}
	}
	return false
}

// orderSpoken is one order as said: table 4, or takeaway number 12.
type orderSpoken struct {
	takeaway bool
	ref      string
}

// spokenOrders reads every order named: "tables 4 and 7", "table 4 and
// table 14", "takeaway 3".
func (p *parser) spokenOrders() []orderSpoken {
	var out []orderSpoken
	for i := 0; i < len(p.w); i++ {
		w := p.w[i]
		if !slices.Contains(orderWords, w) {
			continue
		}
		takeaway := w == "takeaway" || w == "takeaways" || w == "ticket" || w == "number"
		j := i + 1
		if j < len(p.w) && (p.w[j] == "number" || p.w[j] == "no") {
			j++
		}
		for j < len(p.w) {
			ref, ok := tableWord(p.w[j])
			if !ok {
				break
			}
			out = append(out, orderSpoken{takeaway: takeaway, ref: ref})
			j++
			if j+1 < len(p.w) && (p.w[j] == "and" || p.w[j] == "n") {
				if _, ok := tableWord(p.w[j+1]); ok {
					j++
					continue
				}
			}
			break
		}
		i = j - 1
	}
	return out
}

// tableWord reads a table name: "4", "12a", or a word that sounds like a
// number ("for").
func tableWord(w string) (string, bool) {
	if d, ok := soundsLike[w]; ok {
		return d, true
	}
	if w != "" && w[0] >= '0' && w[0] <= '9' && len(w) <= 6 && !strings.Contains(w, ".") {
		return w, true
	}
	return "", false
}

// resolveOrders finds the open orders named. When a table is not open but
// sounds like one or more that are, it returns a question.
func (p *parser) resolveOrders() ([]string, *Result) {
	spoken := p.spokenOrders()
	if len(spoken) == 0 {
		return nil, &Result{Problem: fmt.Sprintf("Heard %q, but not which table. Say the table number, such as \"table 4\".", p.words)}
	}
	var ids []string
	for i, s := range spoken {
		if o, ok := p.exactOrder(s); ok {
			ids = append(ids, o.ID)
			continue
		}
		near := p.nearOrders(s)
		if len(near) == 0 {
			what := "table " + s.ref
			if s.takeaway {
				what = "takeaway " + s.ref
			}
			return nil, &Result{Problem: fmt.Sprintf("There is no open order for %s.", what)}
		}
		// Ask which one, keeping the rest of what was said.
		var opts []Option
		for _, o := range near {
			opts = append(opts, Option{Label: o.Label, Command: Command{OrderIDs: append(append(slices.Clone(ids), o.ID), p.restIDs(spoken[i+1:])...)}})
		}
		labels := make([]string, len(near))
		for k, o := range near {
			labels[k] = o.Label
		}
		ask := orList(labels) + "?"
		if len(near) == 1 {
			ask = "Did you mean " + near[0].Label + "?"
		}
		return nil, &Result{Ask: ask, Options: opts}
	}
	return ids, nil
}

// restIDs resolves the orders after an unclear one, skipping any that are
// also unclear.
func (p *parser) restIDs(rest []orderSpoken) []string {
	var out []string
	for _, s := range rest {
		if o, ok := p.exactOrder(s); ok {
			out = append(out, o.ID)
		}
	}
	return out
}

func (p *parser) exactOrder(s orderSpoken) (OrderRef, bool) {
	for _, o := range p.v.Orders {
		if !s.takeaway && o.Table != "" && strings.EqualFold(o.Table, s.ref) {
			return o, true
		}
	}
	for _, o := range p.v.Orders {
		if (s.takeaway || o.Table == "") && strconv.Itoa(o.Number) == s.ref {
			return o, true
		}
	}
	return OrderRef{}, false
}

// nearOrders are open tables that a misheard number could be: "41" for 4
// or 14, "40" for 14.
func (p *parser) nearOrders(s orderSpoken) []OrderRef {
	var out []OrderRef
	for _, o := range p.v.Orders {
		name := strings.ToLower(o.Table)
		if s.takeaway || name == "" {
			name = strconv.Itoa(o.Number)
		}
		switch {
		case strings.Contains(s.ref, name), strings.Contains(name, s.ref), distance(name, s.ref) <= 1:
			out = append(out, o)
		case teen(s.ref) == name || teen(name) == s.ref, reverse(s.ref) == name:
			out = append(out, o)
		}
	}
	return out
}

// teen maps "40" to "14", "60" to "16" and so on: "forty" and "fourteen"
// sound alike.
func teen(n string) string {
	if len(n) == 2 && n[1] == '0' && n[0] >= '3' && n[0] <= '9' {
		return "1" + n[:1]
	}
	return ""
}

func reverse(s string) string {
	r := []rune(s)
	slices.Reverse(r)
	return string(r)
}

func orList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

// --- courses ---

var courseWords = map[string]string{
	"starter": "starter", "starters": "starter", "starts": "starter", "start": "starter", "apps": "starter", "appetisers": "starter", "appetizers": "starter",
	"main": "main", "mains": "main", "maine": "main", "mane": "main", "manes": "main",
	"dessert": "dessert", "desserts": "dessert", "desert": "dessert", "deserts": "dessert", "sweets": "dessert", "sweet": "dessert", "pudding": "dessert",
}

func (p *parser) spokenCourse(from int) string {
	for _, w := range p.w[from:] {
		if c, ok := courseWords[w]; ok {
			return c
		}
	}
	return ""
}

// course reads "fire table 4 mains" and the like. The course is optional:
// the service picks the next one.
func (p *parser) course(i Intent, from int) Result {
	ids, ask := p.resolveOrders()
	if ask != nil {
		return p.withIntent(*ask, i, p.spokenCourse(from))
	}
	c := p.cmd(i)
	c.OrderIDs, c.Course = ids, p.spokenCourse(from)
	return Result{Command: c}
}

// withIntent fills in a question's options with the rest of the command.
func (p *parser) withIntent(r Result, i Intent, course string) Result {
	for k := range r.Options {
		r.Options[k].Command.Intent = i
		r.Options[k].Command.Course = course
		r.Options[k].Command.Words = p.words
	}
	return r
}

// --- names ---

type nameMatch struct {
	ID, Name   string
	idx        int // position in the list given
	start, end int // span of words matched
	hits       int // name words matched
	of         int // name words
}

// bestNames finds the names that the words match best. Several come back
// when they match equally well.
func bestNames(words []string, names []Name, skip func(int) bool) []nameMatch {
	var all []nameMatch
	for idx, n := range names {
		nw := normalize(n.Name)
		if len(nw) == 0 {
			continue
		}
		best := nameMatch{ID: n.ID, Name: n.Name, of: len(nw), idx: idx}
		for i := range words {
			if skip != nil && skip(i) {
				continue
			}
			// Match name words in order from here.
			hits, k, end := 0, 0, i
			for j := i; j < len(words) && k < len(nw) && j-i < len(nw)+1; j++ {
				if skip != nil && skip(j) {
					break
				}
				matched := false
				for kk := k; kk < len(nw); kk++ {
					if wordMatch(words[j], nw[kk]) {
						hits++
						k = kk + 1
						end = j + 1
						matched = true
						break
					}
				}
				if !matched {
					break
				}
			}
			if hits > best.hits {
				best.hits, best.start, best.end = hits, i, end
			}
		}
		if best.hits > 0 {
			all = append(all, best)
		}
	}
	if len(all) == 0 {
		return nil
	}
	// Most name words matched wins. Among those, a name said in full beats
	// one said in part ("burger" is Burger, not Chicken burger); otherwise
	// they tie, and the caller asks ("fridge" could be any fridge).
	slices.SortStableFunc(all, func(a, b nameMatch) int {
		if a.hits != b.hits {
			return b.hits - a.hits
		}
		return (a.of - a.hits) - (b.of - b.hits)
	})
	top := all[0]
	full := top.hits == top.of
	var out []nameMatch
	for _, m := range all {
		if m.hits == top.hits && (!full || m.hits == m.of) {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(a, b nameMatch) int { return a.idx - b.idx })
	return out
}

// skipWords marks words that are part of the grammar, not a name.
func (p *parser) skipWords() func(int) bool {
	grammar := map[string]bool{}
	for _, w := range append(append(slices.Clone(orderWords), "and", "on", "start", "started", "going", "down", "86", "back", "un", "un86", "eighty", "six"), "for", "fire", "next", "whats", "what", "is", "core", "temp", "temperature", "probe", "cook", "check", "reheat", "reheated", "hot", "hold", "holding", "at") {
		grammar[w] = true
	}
	spoken := map[int]bool{}
	for i := 0; i < len(p.w); i++ {
		if slices.Contains(orderWords, p.w[i]) {
			for j := i + 1; j < len(p.w); j++ {
				if _, ok := tableWord(p.w[j]); ok || p.w[j] == "and" || p.w[j] == "number" {
					spoken[j] = true
					continue
				}
				break
			}
		}
	}
	return func(i int) bool { return grammar[p.w[i]] || spoken[i] }
}

func (p *parser) dish() (nameMatch, *Result) {
	m := bestNames(p.w, p.v.Dishes, p.skipWords())
	switch len(m) {
	case 0:
		return nameMatch{}, &Result{Problem: fmt.Sprintf("Heard %q, but no dish on the menu matches.", p.words)}
	case 1:
		return m[0], nil
	}
	var labels []string
	var opts []Option
	for _, x := range m {
		labels = append(labels, x.Name)
		opts = append(opts, Option{Label: x.Name, Command: Command{DishID: x.ID}})
	}
	return nameMatch{}, &Result{Ask: orList(labels) + "?", Options: opts}
}

// dishOnly reads "86 salmon" or "salmon back on".
func (p *parser) dishOnly(i Intent) Result {
	d, ask := p.dish()
	if ask != nil {
		for k := range ask.Options {
			ask.Options[k].Command.Intent, ask.Options[k].Command.Words = i, p.words
		}
		return *ask
	}
	c := p.cmd(i)
	c.DishID = d.ID
	return Result{Command: c}
}

// next reads "what's next on grill".
func (p *parser) next() Result {
	m := bestNames(p.w, p.v.Stations, nil)
	if len(m) == 0 {
		// "fire" for "fryer": a station called Fryer.
		for _, w := range p.w {
			if w == "fire" || w == "fires" || w == "far" {
				m = bestNames([]string{"fryer"}, p.v.Stations, nil)
			}
		}
	}
	switch len(m) {
	case 0:
		return Result{Problem: fmt.Sprintf("Heard %q, but not which station. Say \"what's next on grill\".", p.words)}
	case 1:
		c := p.cmd(IntentNext)
		c.StationID = m[0].ID
		return Result{Command: c}
	}
	var labels []string
	var opts []Option
	for _, x := range m {
		labels = append(labels, x.Name)
		opts = append(opts, Option{Label: x.Name, Command: Command{Intent: IntentNext, StationID: x.ID, Words: p.words}})
	}
	return Result{Ask: orList(labels) + "?", Options: opts}
}

// start reads "burgers on, tables 4 and 7" and "two burgers on table 4".
func (p *parser) start() Result {
	d, ask := p.dish()
	if ask != nil {
		ids, _ := p.resolveOrders()
		for k := range ask.Options {
			ask.Options[k].Command.Intent, ask.Options[k].Command.Words, ask.Options[k].Command.OrderIDs = IntentStart, p.words, ids
		}
		return *ask
	}
	c := p.cmd(IntentStart)
	c.DishID = d.ID
	if d.start > 0 {
		if n, err := strconv.Atoi(p.w[d.start-1]); err == nil && n > 0 && n <= 50 {
			c.Count = n
		}
	}
	if p.hasOrderRef() {
		ids, ask := p.resolveOrders()
		if ask != nil {
			for k := range ask.Options {
				ask.Options[k].Command.Intent, ask.Options[k].Command.Words, ask.Options[k].Command.DishID = IntentStart, p.words, d.ID
				ask.Options[k].Command.Count = c.Count
			}
			return *ask
		}
		c.OrderIDs = ids
	}
	return Result{Command: c}
}

// --- temperatures ---

// lastNumber returns the last number said, and where.
func (p *parser) lastNumber() (float64, int, bool) {
	for i := len(p.w) - 1; i >= 0; i-- {
		if f, err := strconv.ParseFloat(p.w[i], 64); err == nil {
			return f, i, true
		}
	}
	return 0, -1, false
}

// unitTemp reads "walk-in fridge 3.4 degrees".
func (p *parser) unitTemp() (Result, bool) {
	t, at, ok := p.lastNumber()
	if !ok {
		return Result{}, false
	}
	names := make([]Name, len(p.v.Units))
	for i, u := range p.v.Units {
		names[i] = Name{ID: strconv.Itoa(i), Name: u.Name}
	}
	skip := func(i int) bool { return i == at }
	m := bestNames(p.w, names, skip)
	// A match on "fridge" alone is not enough when there are several
	// fridges; it becomes a question below.
	mentions := p.has("fridge") || p.has("freezer") || p.has("chiller") || p.has("walkin") || p.has("walk")
	if len(m) == 0 && !mentions {
		return Result{}, false
	}
	temp := t
	if len(m) == 1 {
		u := p.v.Units[atoi(m[0].ID)]
		c := p.cmd(IntentUnitTemp)
		c.Unit, c.UnitKind, c.TempC = u.Name, u.Kind, &temp
		return Result{Command: c}, true
	}
	if len(m) == 0 {
		for i, u := range p.v.Units {
			if (p.has("freezer") && u.Kind == "freezer") || (p.has("fridge") && u.Kind == "fridge") {
				m = append(m, nameMatch{ID: strconv.Itoa(i), Name: u.Name})
			}
		}
	}
	if len(m) == 0 {
		return Result{Problem: fmt.Sprintf("Heard %q, but no fridge or freezer on the list matches.", p.words)}, true
	}
	var labels []string
	var opts []Option
	for _, x := range m {
		u := p.v.Units[atoi(x.ID)]
		labels = append(labels, u.Name)
		opts = append(opts, Option{Label: u.Name, Command: Command{Intent: IntentUnitTemp, Unit: u.Name, UnitKind: u.Kind, TempC: &temp, Words: p.words}})
	}
	return Result{Ask: orList(labels) + "?", Options: opts}, true
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// foodTemp reads "core temp chicken 78", "reheat soup 74", "hot hold
// curry 65".
func (p *parser) foodTemp() Result {
	t, at, ok := p.lastNumber()
	if !ok {
		return Result{Problem: fmt.Sprintf("Heard %q, but no temperature. Say \"core temp chicken 78\".", p.words)}
	}
	check := "cook"
	switch {
	case p.has("reheat") || p.has("reheated") || p.has("reheating"):
		check = "reheat"
	case p.hasPhrase("hot hold") || p.has("holding"):
		check = "hot_hold"
	}
	grammar := map[string]bool{"core": true, "temp": true, "temperature": true, "probe": true, "cook": true, "check": true, "cooked": true,
		"reheat": true, "reheated": true, "reheating": true, "hot": true, "hold": true, "holding": true, "at": true, "is": true, "of": true, "for": true}
	var food []string
	for i, w := range p.w {
		if i != at && !grammar[w] {
			food = append(food, w)
		}
	}
	if len(food) == 0 {
		return Result{Problem: fmt.Sprintf("Heard %q, but not which food. Say \"core temp chicken 78\".", p.words)}
	}
	c := p.cmd(IntentFoodTemp)
	c.Check, c.Food, c.TempC = check, strings.Join(food, " "), &t
	// A dish on the menu gives the record its dish and category limits.
	if m := bestNames(food, p.v.Dishes, nil); len(m) == 1 {
		c.DishID = m[0].ID
	}
	return Result{Command: c}
}

// Choose picks an option from a reply to a question: "table 14", "14",
// "the second one", "burger".
func Choose(reply string, options []Option) (int, bool) {
	w := normalize(reply)
	if len(w) == 0 {
		return 0, false
	}
	ordinals := map[string]int{"first": 0, "1st": 0, "second": 1, "2nd": 1, "third": 2, "3rd": 2, "fourth": 3, "4th": 3, "last": len(options) - 1}
	for _, x := range w {
		if i, ok := ordinals[x]; ok && i >= 0 && i < len(options) {
			return i, true
		}
	}
	if len(options) == 1 && slices.Contains(confirmWords, strings.Join(w, " ")) {
		return 0, true
	}
	// Exact label match, then the best name match.
	for i, o := range options {
		if strings.Join(normalize(o.Label), " ") == strings.Join(w, " ") {
			return i, true
		}
	}
	// A bare number picks the option whose label ends in it.
	for _, x := range w {
		if d, ok := soundsLike[x]; ok {
			x = d
		}
		if !isDigits(x) {
			continue
		}
		hit := -1
		for i, o := range options {
			lw := normalize(o.Label)
			if slices.Contains(lw, x) {
				if hit >= 0 {
					return 0, false
				}
				hit = i
			}
		}
		if hit >= 0 {
			return hit, true
		}
	}
	names := make([]Name, len(options))
	for i, o := range options {
		names[i] = Name{ID: strconv.Itoa(i), Name: o.Label}
	}
	if m := bestNames(w, names, nil); len(m) == 1 {
		return atoi(m[0].ID), true
	}
	return 0, false
}
