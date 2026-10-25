package voice

import "time"

// Outcome is how a voice command ended.
type Outcome string

const (
	// OutcomeDone: the command was carried out.
	OutcomeDone Outcome = "done"
	// OutcomeAsked: a question was asked ("Table 4 or table 14?") or a
	// confirmation is needed.
	OutcomeAsked Outcome = "asked"
	// OutcomeCancelled: the person said cancel, or the question expired.
	OutcomeCancelled Outcome = "cancelled"
	// OutcomeNotUnderstood: nothing in the grammar matched.
	OutcomeNotUnderstood Outcome = "not_understood"
	// OutcomeRefused: understood, but the action was refused, such as a
	// course that has already started.
	OutcomeRefused Outcome = "refused"
	// OutcomeAnswered: a question was answered with nothing changed, such
	// as "what's next on grill".
	OutcomeAnswered Outcome = "answered"
)

// Source is how a command arrived.
type Source string

const (
	SourceSpoken Source = "spoken"
	SourceTyped  Source = "typed"
)

// LogEntry records one voice command: what was heard, what it was taken
// to mean, and what happened. Entries are append-only, so a mistake can
// be traced back to what was said.
type LogEntry struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	UserID     string    `json:"user_id"`
	UserName   string    `json:"user_name"`
	Source     Source    `json:"source"`
	Transcript string    `json:"transcript"`
	Intent     Intent    `json:"intent,omitempty"`
	Outcome    Outcome   `json:"outcome"`
	// Reply is what was said back.
	Reply string `json:"reply"`
	// Ref is the order, record or dish the command acted on, if any.
	Ref string `json:"ref,omitempty"`
}
