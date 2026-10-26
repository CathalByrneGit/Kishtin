//go:build sqlite_cgo

package sqlite

import (
	// CGO SQLite driver, selected with -tags sqlite_cgo.
	_ "github.com/mattn/go-sqlite3"
)

// DriverName is the database/sql driver in use.
const DriverName = "sqlite3"

func dsn(path string, readOnly bool) string {
	s := "file:" + path +
		"?_foreign_keys=on" +
		"&_busy_timeout=5000" +
		"&_journal_mode=WAL" +
		"&_synchronous=FULL" +
		"&_txlock=immediate"
	if readOnly {
		s += "&_query_only=1"
	}
	return s
}

// dsnSnapshot opens an existing file read-only without changing it, for
// checking backups.
func dsnSnapshot(path string) string {
	return "file:" + path + "?mode=ro&_query_only=1&_foreign_keys=on"
}
