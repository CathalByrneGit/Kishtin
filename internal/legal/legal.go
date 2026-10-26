// Package legal holds the licence notices built into kishtind and kishtin:
// Kishtin's own licence and NOTICE, and the licences of the Go modules and
// npm packages they include. scripts/licenses writes licenses.txt; `make
// generate` keeps it current.
package legal

import _ "embed"

//go:embed licenses.txt
var notices string

// Notices returns every licence notice, Kishtin's first.
func Notices() string { return notices }
