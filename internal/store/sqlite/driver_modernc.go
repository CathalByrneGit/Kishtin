//go:build !sqlite_cgo

package sqlite

import (
	// Pure-Go SQLite driver, the default.
	_ "modernc.org/sqlite"
)

// DriverName is the database/sql driver in use.
const DriverName = "sqlite"

func dsn(path string, readOnly bool) string {
	s := "file:" + path +
		"?_pragma=foreign_keys(1)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(FULL)" +
		"&_txlock=immediate"
	if readOnly {
		s += "&_pragma=query_only(1)"
	}
	return s
}

// dsnSnapshot opens an existing file read-only without changing it, for
// checking backups.
func dsnSnapshot(path string) string {
	return "file:" + path + "?mode=ro&_pragma=query_only(1)&_pragma=foreign_keys(1)"
}
