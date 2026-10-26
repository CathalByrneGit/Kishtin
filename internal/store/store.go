// Package store defines the storage interfaces every backend implements.
// Core packages depend on these interfaces, never on a database driver.
//
// HACCP records and limit versions have no update or delete methods: they
// are append-only on every backend.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/integrations"
	"github.com/cathalbyrnegit/kishtin/internal/menu"
	"github.com/cathalbyrnegit/kishtin/internal/orders"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/voice"
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
	Settings() SettingsStore
	Menu() MenuStore
	Orders() OrderStore
	Voice() VoiceStore
	POS() POSStore

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
	// DoneFrom and DoneTo bound when the check was done (Record.TakenAt):
	// DoneFrom is inclusive, DoneTo is exclusive.
	DoneFrom, DoneTo time.Time
	Types            []haccp.RecordType
	Outcomes         []haccp.Outcome
	// Limit caps the number of records; 0 means no cap.
	Limit int
	// Newest returns the newest records first instead of oldest first.
	Newest bool
	// ByDone orders records by when the check was done instead of when
	// they were logged.
	ByDone bool
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

// SettingsVersion is one version of a group of settings, such as the
// reference guidance. Versions are never edited: a change appends a new
// version that supersedes the one before it.
type SettingsVersion struct {
	ID         string          `json:"id"`
	Kind       settings.Kind   `json:"kind"`
	Body       json.RawMessage `json:"-"`
	Supersedes string          `json:"supersedes,omitempty"`
	CreatedBy  string          `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
	Reason     string          `json:"reason"`
	PlanRef    string          `json:"plan_ref,omitempty"`
}

// SettingsStore stores settings versions. It has no update or delete.
type SettingsStore interface {
	// AppendSettings returns ErrConflict if the ID is taken or the version
	// it supersedes was already superseded.
	AppendSettings(ctx context.Context, v SettingsVersion) error
	GetSettings(ctx context.Context, id string) (SettingsVersion, error)
	// ListSettings returns every version of a kind, oldest first.
	ListSettings(ctx context.Context, kind settings.Kind) ([]SettingsVersion, error)
}

// MenuStore stores stations, equipment and dishes. Nothing is deleted:
// orders refer to them, so they are marked inactive instead.
type MenuStore interface {
	// CreateStation returns ErrConflict if the name is taken.
	CreateStation(ctx context.Context, s menu.Station) error
	// UpdateStation returns ErrNotFound if the station does not exist.
	UpdateStation(ctx context.Context, s menu.Station) error
	// ListStations returns every station, ordered by position then name.
	ListStations(ctx context.Context) ([]menu.Station, error)

	// CreateEquipment returns ErrConflict if the station already has
	// equipment with that name.
	CreateEquipment(ctx context.Context, e menu.Equipment) error
	UpdateEquipment(ctx context.Context, e menu.Equipment) error
	ListEquipment(ctx context.Context) ([]menu.Equipment, error)

	// CreateDish returns ErrConflict if the name is taken.
	CreateDish(ctx context.Context, d menu.Dish) error
	UpdateDish(ctx context.Context, d menu.Dish) error
	GetDish(ctx context.Context, id string) (menu.Dish, error)
	// ListDishes returns every dish, ordered by name.
	ListDishes(ctx context.Context) ([]menu.Dish, error)
}

// OrderFilter selects orders. With OpenOnly unset, From and To bound
// CreatedAt (From inclusive, To exclusive) and both must be set.
type OrderFilter struct {
	OpenOnly bool
	From, To time.Time
}

// OrderStore stores orders with their courses and items, and the order
// event log. Events are append-only.
type OrderStore interface {
	// CreateOrder stores an order with its courses and items.
	CreateOrder(ctx context.Context, o orders.Order) error
	// UpdateOrder replaces the order's own fields (not courses or items).
	UpdateOrder(ctx context.Context, o orders.Order) error
	// GetOrder returns an order with its courses and items.
	GetOrder(ctx context.Context, id string) (orders.Order, error)
	// ListOrders returns orders with their courses and items, oldest
	// first.
	ListOrders(ctx context.Context, f OrderFilter) ([]orders.Order, error)
	// MaxOrderNumberSince returns the highest ticket number of orders
	// created at or after t, or 0.
	MaxOrderNumberSince(ctx context.Context, t time.Time) (int, error)

	// SaveCourse creates or replaces a course.
	SaveCourse(ctx context.Context, c orders.Course) error
	AddItem(ctx context.Context, it orders.Item) error
	// UpdateItem replaces an item's course, modifiers, notes, state and
	// times. Its dish snapshot never changes.
	UpdateItem(ctx context.Context, it orders.Item) error

	AppendEvent(ctx context.Context, e orders.Event) error
	// ListEvents returns an order's events, oldest first.
	ListEvents(ctx context.Context, orderID string) ([]orders.Event, error)
	// ListItemEvents returns item state changes between from (inclusive)
	// and to (exclusive), oldest first.
	ListItemEvents(ctx context.Context, from, to time.Time) ([]orders.Event, error)
}

// VoiceStore stores the voice command log. Entries are append-only.
type VoiceStore interface {
	AppendVoiceLog(ctx context.Context, e voice.LogEntry) error
	// ListVoiceLog returns entries before the given time (all when zero),
	// newest first, at most limit of them.
	ListVoiceLog(ctx context.Context, before time.Time, limit int) ([]voice.LogEntry, error)
}

// POSStore stores how Kishtin reads from a point-of-sale system: catalog
// mappings, the till orders it has seen, and where polling has got to.
type POSStore interface {
	// SaveMapping creates or replaces the mapping for a catalog entry.
	SaveMapping(ctx context.Context, m integrations.Mapping) error
	ListMappings(ctx context.Context, pos string) ([]integrations.Mapping, error)

	// GetImported returns ErrNotFound for an order not seen before.
	GetImported(ctx context.Context, pos, externalID string) (integrations.Imported, error)
	// SaveImported creates or replaces a till order's record.
	SaveImported(ctx context.Context, o integrations.Imported) error
	// ListImported returns the most recently seen orders first.
	ListImported(ctx context.Context, pos string, limit int) ([]integrations.Imported, error)

	// GetPOSState returns ErrNotFound before the first poll.
	GetPOSState(ctx context.Context, pos string) (integrations.SyncState, error)
	SavePOSState(ctx context.Context, st integrations.SyncState) error
}
