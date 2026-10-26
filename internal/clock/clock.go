// Package clock lets code read the time through an interface, so tests can
// control it.
package clock

import (
	"sync"
	"time"

	// Embed the time zone database so Europe/Dublin works on minimal machines.
	_ "time/tzdata"
)

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// Real reads the system clock. Times are returned in UTC.
type Real struct{}

// Now returns the current UTC time.
func (Real) Now() time.Time { return time.Now().UTC() }

// Fixed is a manually controlled clock for tests.
type Fixed struct {
	mu sync.Mutex
	t  time.Time
}

// NewFixed returns a clock stopped at t.
func NewFixed(t time.Time) *Fixed { return &Fixed{t: t.UTC()} }

// Now returns the clock's current time.
func (f *Fixed) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

// Advance moves the clock forward by d.
func (f *Fixed) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

// Set moves the clock to t.
func (f *Fixed) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = t.UTC()
}

// Dublin is the display time zone for the restaurant.
var Dublin = mustLoad("Europe/Dublin")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic("load time zone " + name + ": " + err.Error())
	}
	return loc
}
