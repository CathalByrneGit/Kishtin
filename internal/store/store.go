// Package store defines the storage interfaces every backend implements.
// Core packages depend on these interfaces, never on a database driver.
//
// HACCP records and limit versions have no update or delete methods: they
// are append-only on every backend.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a write breaks a uniqueness rule, such as a
// duplicate name or a second record with the same sequence number.
var ErrConflict = errors.New("conflict")

// Store groups every storage interface.
type Store interface {
	HACCP() HACCPStore
	Users() UserStore
	Auth() AuthStore
	Files() FileStore

	// WithTx runs fn in one transaction. If fn returns an error the
	// transaction is rolled back. Writes inside fn must use tx.
	WithTx(ctx context.Context, fn func(tx Store) error) error

	Close() error
}

// Backuper is implemented by backends that can write a consistent copy of
// themselves to a file.
type Backuper interface {
	// Backup writes a consistent snapshot to dest, which must not exist.
	Backup(ctx context.Context, dest string) error
}

// RecordFilter selects HACCP records. Zero fields don't filter.
type RecordFilter struct {
	// From and To bound LoggedAt: From is inclusive, To is exclusive.
	From, To time.Time
	Types    []haccp.RecordType
	// Limit caps the number of records; 0 means no cap.
	Limit int
	// Newest returns the newest records first instead of oldest first.
	Newest bool
}

// HACCPStore stores HACCP records and limit versions.
type HACCPStore interface {
	// AppendRecord stores a sealed record. It returns ErrConflict if the
	// sequence number or hash is already used. Callers seal the record and
	// append it inside one transaction, so the chain cannot fork.
	AppendRecord(ctx context.Context, r haccp.Record) error
	// LastRecord returns the record with the highest sequence number, or
	// nil if there are none.
	LastRecord(ctx context.Context) (*haccp.Record, error)
	GetRecord(ctx context.Context, id string) (haccp.Record, error)
	ListRecords(ctx context.Context, f RecordFilter) ([]haccp.Record, error)
	// EachRecord calls fn for every record in sequence order. It stops at
	// the first error fn returns.
	EachRecord(ctx context.Context, fn func(haccp.Record) error) error

	AppendLimitVersion(ctx context.Context, lv haccp.LimitVersion) error
	GetLimitVersion(ctx context.Context, id string) (haccp.LimitVersion, error)
	// ListLimitVersions returns every version, oldest first.
	ListLimitVersions(ctx context.Context) ([]haccp.LimitVersion, error)
}

// UserStore stores staff.
type UserStore interface {
	// CreateUser returns ErrConflict if the name is taken.
	CreateUser(ctx context.Context, u auth.User) error
	// UpdateUser replaces a user's name, role, active flag and hashes.
	UpdateUser(ctx context.Context, u auth.User) error
	GetUser(ctx context.Context, id string) (auth.User, error)
	// ListUsers returns every user, ordered by name.
	ListUsers(ctx context.Context) ([]auth.User, error)
}

// AuthStore stores sessions and API keys.
type AuthStore interface {
	CreateSession(ctx context.Context, s auth.Session) error
	GetSession(ctx context.Context, tokenHash string) (auth.Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	// DeleteExpiredSessions removes sessions that expired before now.
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)

	CreateAPIKey(ctx context.Context, k auth.APIKey) error
	GetAPIKeyByHash(ctx context.Context, tokenHash string) (auth.APIKey, error)
	ListAPIKeys(ctx context.Context) ([]auth.APIKey, error)
	RevokeAPIKey(ctx context.Context, id string, at time.Time) error
}

// FileMeta describes an attached file. The file itself lives on disk under
// its SHA-256.
type FileMeta struct {
	SHA256       string    `json:"sha256"`
	MIME         string    `json:"mime"`
	Size         int64     `json:"size"`
	OriginalName string    `json:"original_name,omitempty"`
	UploadedBy   string    `json:"uploaded_by"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

// FileStore stores attachment metadata. Files are never deleted.
type FileStore interface {
	// PutFile stores metadata. Storing the same hash again is not an error
	// and keeps the first upload's metadata.
	PutFile(ctx context.Context, m FileMeta) error
	GetFile(ctx context.Context, sha256 string) (FileMeta, error)
	ListFiles(ctx context.Context) ([]FileMeta, error)
}
