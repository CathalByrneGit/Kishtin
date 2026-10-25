package voice

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Intents for voids and the HACCP records that are logged in steps. Any
// detail not said is asked for, one question at a time.
const (
	// IntentVoid voids items: "void burger table 4". It always needs a
	// reason and a spoken "confirm void".
	IntentVoid Intent = "void"
	// IntentDelivery logs a delivery: "delivery chilled chicken from
	// Synthetic Farm 3 degrees".
	IntentDelivery Intent = "delivery"
	// IntentCooling logs food going into the fridge to cool: "cooling rice
	// cooked at 14:30".
	IntentCooling Intent = "cooling"
	// IntentBatch logs a batch label: "batch grape syrup 5 days".
	IntentBatch Intent = "batch"
	// IntentCleaning ticks a cleaning task: "floors done".
	IntentCleaning Intent = "cleaning"
)

// Cooked is when food was cooked, as said: a clock time ("14:30") or how
// long ago ("90 minutes ago"). The caller turns it into a time, because
// this package does not read the clock.
type Cooked struct {
	// Clock is "HH:MM" on the 24-hour clock.
	Clock string `json:"clock,omitempty"`
	// Ago is how long before now.
	Ago time.Duration `json:"ago,omitempty"`
}

// --- voids ---

var voidWords = []string{"void", "voided", "bin", "binned", "scrap", "waste", "wasted"}

// void reads "void burger table 4", "void 2 burgers table 4 because
// dropped". The reason is asked for if not said.
func (p *parser) void() Result {
	reason := ""
	words := p.w
	for i, w := range words {
		if w == "because" || w == "reason" {
			reason = sentenceCase(strings.Join(words[i+1:], " "))
			p.w = words[:i]
			break
		}
	}
	defer func() { p.w = words }()
	if !p.hasOrderRef() {
		return Result{Problem: fmt.Sprintf("Heard %q, but not which table. Say \"void burger table 4\".", p.words)}
	}
	d, ask := p.dish()
	if ask != nil {
		ids, _ := p.resolveOrders()
		for k := range ask.Options {
			c := &ask.Options[k].Command
			c.Intent, c.Words, c.OrderIDs, c.Reason = IntentVoid, p.words, ids, reason
		}
		return *ask
	}
	ids, ask := p.resolveOrders()
	if ask != nil {
		for k := range ask.Options {
			c := &ask.Options[k].Command
			c.Intent, c.Words, c.DishID, c.Reason = IntentVoid, p.words, d.ID, reason
		}
		return *ask
	}
	c := p.cmd(IntentVoid)
	c.OrderIDs, c.DishID, c.Reason = ids, d.ID, reason
	if d.start > 0 {
		if n, err := strconv.Atoi(p.w[d.start-1]); err == nil && n > 0 && n <= 50 {
			c.Count = n
		}
	}
	return Result{Command: c}
}

// --- deliveries ---

var deliveryKinds = map[string]string{
	"chilled": "chilled", "chill": "chilled", "fridge": "chilled", "fresh": "chilled", "cold": "chilled",
	"frozen": "frozen", "freezer": "frozen",
	"ambient": "ambient", "dry": "ambient", "dried": "ambient", "tinned": "ambient", "room": "ambient",
}

// delivery reads "delivery chilled chicken from Synthetic Farm 3 degrees,
// packaging fine, in date". Anything not said is asked for.
func (p *parser) delivery() Result {
	c := p.cmd(IntentDelivery)
	grammar := map[string]bool{"delivery": true, "deliveries": true, "delivered": true, "of": true, "at": true, "temp": true, "temperature": true, "is": true}
	var product, supplier []string
	inFrom := false
	numAt := -1
	if t, at, ok := p.lastNumber(); ok {
		c.TempC, numAt = &t, at
	}
	for i := 0; i < len(p.w); i++ {
		w := p.w[i]
		switch {
		case i == numAt:
			inFrom = false
		case p.yesNoPhrase(i) > 0:
			i += p.yesNoPhrase(i) - 1
			inFrom = false
		case w == "from":
			inFrom = true
		case deliveryKinds[w] != "" && c.DeliveryKind == "":
			c.DeliveryKind = deliveryKinds[w]
			inFrom = false
		case grammar[w]:
		case inFrom:
			supplier = append(supplier, w)
		default:
			product = append(product, w)
		}
	}
	c.Supplier = titleCase(strings.Join(supplier, " "))
	c.Product = sentenceCase(strings.Join(product, " "))
	// "Packaging fine" and "in date" can be said up front.
	if p.hasPhrase("packaging ok") || p.hasPhrase("packaging fine") || p.hasPhrase("packaging good") || p.hasPhrase("packaging intact") {
		c.PackagingOK = ptr(true)
	}
	if p.hasPhrase("packaging damaged") || p.hasPhrase("packaging torn") || p.hasPhrase("packaging broken") {
		c.PackagingOK = ptr(false)
	}
	if p.hasPhrase("in date") {
		c.InDate = ptr(true)
	}
	if p.hasPhrase("out of date") {
		c.InDate = ptr(false)
	}
	for _, v := range []string{"van", "vehicle", "truck", "lorry"} {
		for _, ok := range []string{"ok", "fine", "good", "clean", "cold"} {
			if p.hasPhrase(v + " " + ok) {
				c.VehicleOK = ptr(true)
			}
		}
		for _, bad := range []string{"dirty", "warm", "failed", "bad"} {
			if p.hasPhrase(v + " " + bad) {
				c.VehicleOK = ptr(false)
			}
		}
	}
	if c.DeliveryKind == "ambient" {
		c.TempC = nil
	}
	return Result{Command: c}
}

// yesNoPhrase returns how many words a packaging, date or vehicle phrase at i
// uses, so they are not taken as the product.
func (p *parser) yesNoPhrase(i int) int {
	rest := strings.Join(p.w[i:], " ") + " "
	for _, ph := range []string{"packaging ok ", "packaging fine ", "packaging good ", "packaging intact ", "packaging damaged ", "packaging torn ", "packaging broken ", "out of date ", "in date "} {
		if strings.HasPrefix(rest, ph) {
			return len(strings.Fields(ph))
		}
	}
	for _, v := range []string{"van ", "vehicle ", "truck ", "lorry "} {
		for _, st := range []string{"ok ", "fine ", "good ", "clean ", "cold ", "dirty ", "warm ", "failed ", "bad "} {
			if strings.HasPrefix(rest, v+st) {
				return 2
			}
		}
	}
	return 0
}

// --- cooling ---

// cooling reads "cooling rice cooked at 14:30" or "rice into the fridge,
// cooked 90 minutes ago". The food is going into the fridge now.
func (p *parser) cooling() Result {
	c := p.cmd(IntentCooling)
	cooked, from, to := readCooked(p.w)
	if cooked != nil {
		c.Cooked = cooked
	}
	grammar := map[string]bool{"cooling": true, "cooled": true, "cool": true, "into": true, "in": true, "fridge": true, "blast": true,
		"chiller": true, "chill": true, "now": true, "going": true, "gone": true, "put": true, "of": true, "to": true}
	var food []string
	for i, w := range p.w {
		if (i >= from && i < to) || grammar[w] {
			continue
		}
		food = append(food, w)
	}
	c.Food = sentenceCase(strings.Join(food, " "))
	return Result{Command: c}
}

// readCooked finds "cooked at 14 30", "cooked at 2 30 pm", "cooked 90
// minutes ago" or "cooked an hour ago" and returns it with the span of
// words it used.
func readCooked(w []string) (*Cooked, int, int) {
	start := slices.Index(w, "cooked")
	if start < 0 {
		start = slices.Index(w, "made")
	}
	if start < 0 {
		return nil, -1, -1
	}
	i := start + 1
	if i < len(w) && w[i] == "at" {
		i++
	}
	if c, n, ok := readWhen(w[i:]); ok {
		return &c, start, i + n
	}
	return nil, -1, -1
}

// readWhen reads a time at the start of w: a clock time or a time ago.
func readWhen(w []string) (Cooked, int, bool) {
	if len(w) == 0 {
		return Cooked{}, 0, false
	}
	// "90 minutes ago", "hour ago", "2 hours ago", "half hour ago"
	n, used := 1.0, 0
	if v, err := strconv.ParseFloat(w[0], 64); err == nil {
		n, used = v, 1
	} else if w[0] == "half" {
		n, used = 0.5, 1
	}
	if used < len(w) {
		unit := w[used]
		mult := time.Duration(0)
		switch unit {
		case "minute", "minutes", "min", "mins":
			mult = time.Minute
		case "hour", "hours", "hr", "hrs":
			mult = time.Hour
		}
		if mult > 0 && used+1 < len(w) && w[used+1] == "ago" {
			return Cooked{Ago: time.Duration(n * float64(mult))}, used + 2, true
		}
	}
	// "14 30", "1430", "2 30 pm", "2 pm"
	h, m, k := -1, 0, 0
	if v, err := strconv.Atoi(w[0]); err == nil {
		switch {
		case v >= 100 && v <= 2359:
			h, m, k = v/100, v%100, 1
		case v >= 0 && v <= 23:
			h, k = v, 1
			if len(w) > 1 {
				if mm, err := strconv.Atoi(w[1]); err == nil && mm >= 0 && mm < 60 {
					m, k = mm, 2
				}
			}
		}
	}
	if h < 0 || m > 59 {
		return Cooked{}, 0, false
	}
	if k < len(w) {
		switch w[k] {
		case "pm":
			if h < 12 {
				h += 12
			}
			k++
		case "am":
			if h == 12 {
				h = 0
			}
			k++
		}
	}
	if h > 23 {
		return Cooked{}, 0, false
	}
	return Cooked{Clock: fmt.Sprintf("%02d:%02d", h, m)}, k, true
}

// --- batch labels ---

var quantityUnits = []string{"litre", "litres", "liter", "liters", "l", "kilo", "kilos", "kg", "grams", "g", "portions", "portion", "tubs", "tub", "bottles", "bottle", "trays", "tray", "jars", "jar", "pots", "pot"}

// batch reads "batch grape syrup 5 days" or "label 2 litres of stock 3
// days". The shelf life is asked for if not said.
func (p *parser) batch() Result {
	c := p.cmd(IntentBatch)
	skip := map[int]bool{}
	for i := 0; i+1 < len(p.w); i++ {
		if _, err := strconv.ParseFloat(p.w[i], 64); err != nil {
			continue
		}
		switch {
		case p.w[i+1] == "days" || p.w[i+1] == "day":
			n, _ := strconv.Atoi(p.w[i])
			c.ShelfDays = n
			skip[i], skip[i+1] = true, true
		case slices.Contains(quantityUnits, p.w[i+1]):
			c.Quantity = p.w[i] + " " + p.w[i+1]
			skip[i], skip[i+1] = true, true
		}
	}
	grammar := map[string]bool{"batch": true, "label": true, "labelled": true, "made": true, "of": true, "keeps": true, "for": true, "use": true, "by": true, "shelf": true, "life": true}
	var product []string
	for i, w := range p.w {
		if !skip[i] && !grammar[w] {
			product = append(product, w)
		}
	}
	c.Product = sentenceCase(strings.Join(product, " "))
	return Result{Command: c}
}

// --- cleaning ---

var cleaningWords = []string{"clean", "cleaned", "cleaning", "done", "finished", "complete", "completed", "wiped", "mopped", "swept", "emptied", "sanitised", "sanitized"}

// cleaning reads "floors done" or "cleaning bins emptied". It matches the
// cleaning task list.
func (p *parser) cleaning() (Result, bool) {
	if !slices.ContainsFunc(p.w, func(w string) bool { return slices.Contains(cleaningWords, w) }) {
		return Result{}, false
	}
	names := make([]Name, len(p.v.Tasks))
	for i, t := range p.v.Tasks {
		names[i] = Name{ID: strconv.Itoa(i), Name: t}
	}
	generic := map[string]bool{"clean": true, "cleaned": true, "cleaning": true, "done": true, "finished": true, "complete": true, "completed": true}
	words := make([]string, 0, len(p.w))
	for _, w := range p.w {
		if !generic[w] {
			words = append(words, w)
		}
	}
	m := bestNames(words, names, nil)
	switch {
	case len(m) == 0 && slices.Contains(p.w, "cleaning"):
		return Result{Command: p.cmd(IntentCleaning)}, true
	case len(m) == 0:
		return Result{}, false
	case len(m) == 1:
		c := p.cmd(IntentCleaning)
		c.Task = m[0].Name
		return Result{Command: c}, true
	}
	var labels []string
	var opts []Option
	for _, x := range m {
		labels = append(labels, x.Name)
		opts = append(opts, Option{Label: x.Name, Command: Command{Intent: IntentCleaning, Task: x.Name, Words: p.words}})
	}
	return Result{Ask: orList(labels) + "?", Options: opts}, true
}

// --- answers to questions ---

// ReadYesNo reads a yes or no answer: "yes", "all good", "no, the box was
// torn".
func ReadYesNo(text string) (bool, bool) {
	w := normalize(text)
	if len(w) == 0 {
		return false, false
	}
	switch w[0] {
	case "yes", "yeah", "yep", "yup", "fine", "good", "grand", "ok", "correct", "intact", "all":
		return true, true
	case "no", "nope", "not", "damaged", "torn", "broken", "out":
		return false, true
	}
	return false, false
}

// ReadNumber reads the last number said: "3 degrees", "minus eighteen".
func ReadNumber(text string) (float64, bool) {
	p := parser{w: normalize(text)}
	t, _, ok := p.lastNumber()
	return t, ok
}

// ReadDeliveryKind reads chilled, frozen or ambient.
func ReadDeliveryKind(text string) (string, bool) {
	for _, w := range normalize(text) {
		if k, ok := deliveryKinds[w]; ok {
			return k, true
		}
	}
	return "", false
}

// ReadCooked reads when food was cooked: "14 30", "2 pm", "90 minutes
// ago", "cooked at 14 30".
func ReadCooked(text string) (Cooked, bool) {
	w := normalize(text)
	if c, _, _ := readCooked(w); c != nil {
		return *c, true
	}
	if len(w) > 0 && w[0] == "at" {
		w = w[1:]
	}
	c, _, ok := readWhen(w)
	return c, ok
}

// ReadDays reads a shelf life: "5", "five days", "a week".
func ReadDays(text string) (int, bool) {
	w := normalize(text)
	for i, x := range w {
		if x == "week" || x == "weeks" {
			n := 1
			if i > 0 {
				if v, err := strconv.Atoi(w[i-1]); err == nil {
					n = v
				}
			}
			return 7 * n, true
		}
	}
	for _, x := range w {
		if v, err := strconv.Atoi(x); err == nil && v >= 0 && v <= 365 {
			return v, true
		}
	}
	return 0, false
}

// ReadText cleans a free answer, such as a supplier or a reason.
func ReadText(text string) string {
	return sentenceCase(strings.TrimRight(strings.TrimSpace(text), ".!?"))
}

// ChooseTask picks a cleaning task from a reply.
func ChooseTask(reply string, tasks []string) (string, bool) {
	names := make([]Name, len(tasks))
	for i, t := range tasks {
		names[i] = Name{ID: strconv.Itoa(i), Name: t}
	}
	if m := bestNames(normalize(reply), names, nil); len(m) == 1 {
		return m[0].Name, true
	}
	return "", false
}

func sentenceCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	return string(r)
}

func titleCase(s string) string {
	f := strings.Fields(s)
	for i, w := range f {
		f[i] = sentenceCase(w)
	}
	return strings.Join(f, " ")
}

func ptr[T any](v T) *T { return &v }
