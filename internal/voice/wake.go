package voice

import "strings"

// OutcomeIgnored is speech heard in hands-free mode that did not start
// with the wake word. It is not acted on, logged or echoed back.
const OutcomeIgnored Outcome = "ignored"

// DefaultWakeWords is what starts a hands-free command.
var DefaultWakeWords = []string{"kishtin"}

// StripWake checks that a transcript starts with a wake word, allowing
// for how a speech model may write it ("Kish tin", "Kishteen"), and
// returns what was said after it.
func StripWake(transcript string, wakeWords []string) (string, bool) {
	w := normalize(transcript)
	start := 0
	if len(w) > 0 && (w[0] == "hey" || w[0] == "hi" || w[0] == "right") {
		start = 1
	}
	for _, wake := range wakeWords {
		target := strings.Join(normalize(wake), "")
		if target == "" {
			continue
		}
		for n := 1; n <= 3 && start+n <= len(w); n++ {
			said := strings.Join(w[start:start+n], "")
			slips := allowedSlips(target)
			if n > 1 {
				// Joined words ("kish tin") must be closer, or two
				// ordinary words could add up to the wake word.
				slips = min(slips, 1)
			}
			if said == target || distance(said, target) <= slips {
				return strings.Join(w[start+n:], " "), true
			}
		}
	}
	return "", false
}

// allowedSlips is how many letters a heard wake word may differ by.
// Short words must match exactly, or every passing word would wake it.
func allowedSlips(target string) int {
	switch n := len([]rune(target)); {
	case n >= 6:
		return 2
	case n >= 4:
		return 1
	}
	return 0
}
