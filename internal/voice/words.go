package voice

import (
	"strconv"
	"strings"
	"unicode"
)

// normalize lowercases a transcript and splits it into words. Punctuation
// is dropped, except a decimal point or comma between digits ("3.4",
// "3,4") and a leading minus ("-18"). Hyphens split words, so "walk-in"
// is "walk in" and "eighty-six" is "eighty six".
func normalize(s string) []string {
	s = strings.ToLower(s)
	r := []rune(s)
	var b strings.Builder
	for i, c := range r {
		digitBefore := i > 0 && unicode.IsDigit(r[i-1])
		digitAfter := i+1 < len(r) && unicode.IsDigit(r[i+1])
		switch {
		case unicode.IsLetter(c) || unicode.IsDigit(c):
			b.WriteRune(c)
		case (c == '.' || c == ',') && digitBefore && digitAfter:
			b.WriteRune('.')
		case c == '-' && digitAfter && !digitBefore:
			b.WriteString(" -")
		case c == '\'' || c == '’':
			// "what's" → "whats", "86'd" → "86d"
		default:
			b.WriteRune(' ')
		}
	}
	words := strings.Fields(b.String())
	out := make([]string, 0, len(words))
	for _, w := range words {
		switch w {
		case "degrees", "degree", "celsius", "centigrade", "c", "please", "the", "a", "an", "um", "uh", "er", "okay", "ok":
			continue
		case "86d", "86ed", "eightysix":
			w = "86"
		}
		out = append(out, w)
	}
	return joinNumbers(out)
}

var units = map[string]int{
	"zero": 0, "oh": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9,
	"ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16,
	"seventeen": 17, "eighteen": 18, "nineteen": 19,
}

// tens includes a common misspelling of forty that a speech model may write.
var tens = map[string]int{
	"twenty": 20, "thirty": 30, "forty": 40, "fourty": 40, //nolint:misspell // heard as written
	"fifty": 50, "sixty": 60, "seventy": 70, "eighty": 80, "ninety": 90,
}

// joinNumbers turns number words into digits: "seventy eight" → "78",
// "three point four" → "3.4", "minus eighteen" → "-18".
func joinNumbers(in []string) []string {
	var out []string
	for i := 0; i < len(in); i++ {
		j, neg := i, false
		if (in[j] == "minus" || in[j] == "negative") && j+1 < len(in) && isNumberStart(in[j+1]) {
			j, neg = j+1, true
		}
		s, n, ok := numberAt(in, j)
		if !ok {
			out = append(out, in[i])
			continue
		}
		// "3 point 4", "three point four five"
		if k := j + n; k+1 < len(in) && in[k] == "point" && !strings.Contains(s, ".") {
			var dec strings.Builder
			for k++; k < len(in); k++ {
				if d, ok := units[in[k]]; ok && d < 10 {
					dec.WriteString(strconv.Itoa(d))
				} else if isDigits(in[k]) {
					dec.WriteString(in[k])
				} else {
					break
				}
			}
			if dec.Len() > 0 {
				s += "." + dec.String()
				n = k - j
			}
		}
		if neg && !strings.HasPrefix(s, "-") {
			s = "-" + s
		}
		out = append(out, s)
		i = j + n - 1
	}
	return out
}

// numberAt reads a number at in[i]: digits as written, or number words.
// It returns the digits and how many words it used.
func numberAt(in []string, i int) (string, int, bool) {
	if f, err := strconv.ParseFloat(in[i], 64); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64), 1, true
	}
	v, n, ok := wholeNumber(in, i)
	if !ok {
		return "", 0, false
	}
	return strconv.Itoa(v), n, true
}

func isNumberStart(w string) bool {
	if _, ok := units[w]; ok {
		return true
	}
	if _, ok := tens[w]; ok {
		return true
	}
	_, err := strconv.ParseFloat(w, 64)
	return err == nil
}

// wholeNumber reads number words at in[i] and returns the value and how
// many words it used.
func wholeNumber(in []string, i int) (v, n int, ok bool) {
	w := in[i]
	if t, ok := tens[w]; ok {
		if i+1 < len(in) {
			if u, ok := units[in[i+1]]; ok && u > 0 && u < 10 {
				return t + u, 2, true
			}
		}
		return t, 1, true
	}
	if u, ok := units[w]; ok && w != "oh" {
		return u, 1, true
	}
	return 0, 0, false
}

func isDigits(w string) bool {
	if w == "" {
		return false
	}
	for _, c := range w {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// soundsLike maps words a speech model often writes for a number spoken
// after "table": "table for" is table 4.
var soundsLike = map[string]string{
	"for": "4", "fore": "4", "to": "2", "too": "2", "ate": "8", "won": "1", "tree": "3", "free": "3", "sex": "6",
	"tin": "10", "tan": "10",
}

// singular turns a plural word into its singular for matching: "burgers"
// → "burger", "fries" stays (a dish name is matched both ways).
func singular(w string) string {
	switch {
	case len(w) > 4 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 4 && (strings.HasSuffix(w, "ches") || strings.HasSuffix(w, "shes") || strings.HasSuffix(w, "oes")):
		return w[:len(w)-2]
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		return w[:len(w)-1]
	}
	return w
}

// distance is the Levenshtein distance between two words.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// wordMatch reports whether a spoken word matches a name's word, allowing
// plurals and one wrong letter in longer words.
func wordMatch(spoken, name string) bool {
	if spoken == name || singular(spoken) == singular(name) {
		return true
	}
	if len([]rune(name)) >= 5 && len([]rune(spoken)) >= 4 {
		return distance(singular(spoken), singular(name)) <= 1
	}
	return false
}
