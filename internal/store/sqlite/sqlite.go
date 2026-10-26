// Package sqlite is the default storage backend: one SQLite file in WAL
// mode, with one writer connection and a small pool of read-only readers.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"strings"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/haccp"
	"github.com/cathalbyrnegit/kishtin/internal/settings"
	"github.com/cathalbyrnegit/kishtin/internal/store"
	"github.com/cathalbyrnegit/kishtin/internal/store/sqlite/sqlitedb"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store is the SQLite backend.
type Store struct {
	writer *sql.DB
	reader *sql.DB
	// w runs writes; r runs reads. Inside a transaction both use the
	// transaction, so reads see the transaction's own writes.
	w, r *sqlitedb.Queries
	inTx bool
}

var (
	_ store.Store    = (*Store)(nil)
	_ store.Backuper = (*Store)(nil)
)

// Open opens (creating if needed) the database at path and applies any
// pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.ContainsAny(path, "?#") {
		return nil, fmt.Errorf("open %s: the database path cannot contain ? or #", path)
	}
	writer, err := sql.Open(DriverName, dsn(path, false))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	writer.SetMaxOpenConns(1)
	writer.SetConnMaxIdleTime(0)
	if err := writer.PingContext(ctx); err != nil {
		writer.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := Migrate(ctx, writer); err != nil {
		writer.Close()
		return nil, err
	}
	reader, err := sql.Open(DriverName, dsn(path, true))
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("open %s for reading: %w", path, err)
	}
	reader.SetMaxOpenConns(4)
	return &Store{
		writer: writer,
		reader: reader,
		w:      sqlitedb.New(writer),
		r:      sqlitedb.New(reader),
	}, nil
}

// OpenSnapshot opens an existing database file, such as a backup, without
// writing to it or applying migrations.
func OpenSnapshot(ctx context.Context, path string) (*Store, error) {
	if strings.ContainsAny(path, "?#") {
		return nil, fmt.Errorf("open %s: the database path cannot contain ? or #", path)
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db, err := sql.Open(DriverName, dsnSnapshot(path))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	q := sqlitedb.New(db)
	return &Store{writer: db, reader: db, w: q, r: q}, nil
}

// Migrate applies pending schema migrations.
func Migrate(ctx context.Context, db *sql.DB) error {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, db, sub)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// Close closes both connection pools.
func (s *Store) Close() error {
	if s.reader == s.writer {
		return s.writer.Close()
	}
	return errors.Join(s.reader.Close(), s.writer.Close())
}

func (s *Store) HACCP() store.HACCPStore { return s }
func (s *Store) Users() store.UserStore  { return s }
func (s *Store) Auth() store.AuthStore   { return s }
func (s *Store) Files() store.FileStore  { return s }

// Settings returns the settings store.
func (s *Store) Settings() store.SettingsStore { return s }

// WithTx runs fn in an IMMEDIATE transaction on the writer connection.
// Nested calls join the outer transaction.
func (s *Store) WithTx(ctx context.Context, fn func(tx store.Store) error) error {
	if s.inTx {
		return fn(s)
	}
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	q := sqlitedb.New(tx)
	txs := &Store{writer: s.writer, reader: s.reader, w: q, r: q, inTx: true}
	if err := fn(txs); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(err, fmt.Errorf("roll back: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Backup writes a consistent copy of the database to dest with VACUUM INTO.
func (s *Store) Backup(ctx context.Context, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("back up to %s: file already exists", dest)
	}
	if _, err := s.writer.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		return fmt.Errorf("back up to %s: %w", dest, err)
	}
	return nil
}

// mapErr turns driver errors into store errors.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return store.ErrNotFound
	case strings.Contains(err.Error(), "UNIQUE constraint failed"):
		return fmt.Errorf("%w: %w", store.ErrConflict, err)
	}
	return err
}

func us(t time.Time) int64     { return t.UnixMicro() }
func fromUS(v int64) time.Time { return time.UnixMicro(v).UTC() }
func nullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// --- HACCP records ---

func (s *Store) AppendRecord(ctx context.Context, r haccp.Record) error {
	reading, err := json.Marshal(r.Reading)
	if err != nil {
		return fmt.Errorf("encode reading: %w", err)
	}
	var scope string
	if r.LimitScope != nil {
		b, err := json.Marshal(r.LimitScope)
		if err != nil {
			return fmt.Errorf("encode limit scope: %w", err)
		}
		scope = string(b)
	}
	return mapErr(s.w.InsertRecord(ctx, sqlitedb.InsertRecordParams{
		ID:               r.ID,
		Seq:              r.Seq,
		Type:             string(r.Type),
		LoggedBy:         r.LoggedBy,
		LoggedByName:     r.LoggedByName,
		LoggedAt:         us(r.LoggedAt),
		Reading:          string(reading),
		Outcome:          string(r.Outcome),
		OutcomeReason:    r.OutcomeReason,
		LimitVersionID:   r.LimitVersionID,
		LimitScope:       scope,
		RulesVersionID:   r.RulesVersionID,
		CorrectiveAction: r.CorrectiveAction,
		Corrects:         r.Corrects,
		CorrectionReason: r.CorrectionReason,
		PrevHash:         r.PrevHash,
		Hash:             r.Hash,
		DoneAt:           us(r.TakenAt()),
	}))
}

func recordFromRow(row sqlitedb.HaccpRecord) (haccp.Record, error) {
	r := haccp.Record{
		ID:               row.ID,
		Seq:              row.Seq,
		Type:             haccp.RecordType(row.Type),
		LoggedBy:         row.LoggedBy,
		LoggedByName:     row.LoggedByName,
		LoggedAt:         fromUS(row.LoggedAt),
		Outcome:          haccp.Outcome(row.Outcome),
		OutcomeReason:    row.OutcomeReason,
		LimitVersionID:   row.LimitVersionID,
		RulesVersionID:   row.RulesVersionID,
		CorrectiveAction: row.CorrectiveAction,
		Corrects:         row.Corrects,
		CorrectionReason: row.CorrectionReason,
		PrevHash:         row.PrevHash,
		Hash:             row.Hash,
	}
	if err := json.Unmarshal([]byte(row.Reading), &r.Reading); err != nil {
		return r, fmt.Errorf("decode reading of record %s: %w", row.ID, err)
	}
	if row.LimitScope != "" {
		var sc haccp.Scope
		if err := json.Unmarshal([]byte(row.LimitScope), &sc); err != nil {
			return r, fmt.Errorf("decode limit scope of record %s: %w", row.ID, err)
		}
		r.LimitScope = &sc
	}
	return r, nil
}

func recordsFromRows(rows []sqlitedb.HaccpRecord) ([]haccp.Record, error) {
	out := make([]haccp.Record, 0, len(rows))
	for _, row := range rows {
		r, err := recordFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Store) LastRecord(ctx context.Context) (*haccp.Record, error) {
	row, err := s.r.LastRecord(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r, err := recordFromRow(row)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) GetRecord(ctx context.Context, id string) (haccp.Record, error) {
	row, err := s.r.GetRecord(ctx, id)
	if err != nil {
		return haccp.Record{}, mapErr(err)
	}
	return recordFromRow(row)
}

func (s *Store) ListRecords(ctx context.Context, f store.RecordFilter) ([]haccp.Record, error) {
	from, to := int64(math.MinInt64), int64(math.MaxInt64)
	if !f.From.IsZero() {
		from = us(f.From)
	}
	if !f.To.IsZero() {
		to = us(f.To)
	}
	doneFrom, doneTo := int64(math.MinInt64), int64(math.MaxInt64)
	if !f.DoneFrom.IsZero() {
		doneFrom = us(f.DoneFrom)
	}
	if !f.DoneTo.IsZero() {
		doneTo = us(f.DoneTo)
	}
	types := make([]string, 0, len(f.Types))
	for _, t := range f.Types {
		if !t.Valid() {
			return nil, fmt.Errorf("list records: unknown record type %q", t)
		}
		types = append(types, string(t))
	}
	typesCSV := strings.Join(types, ",")
	outcomes := make([]string, 0, len(f.Outcomes))
	for _, o := range f.Outcomes {
		outcomes = append(outcomes, string(o))
	}
	outcomesCSV := strings.Join(outcomes, ",")
	limit := int64(-1) // SQLite: no limit
	if f.Limit > 0 {
		limit = int64(f.Limit)
	}
	var rows []sqlitedb.HaccpRecord
	var err error
	args := sqlitedb.ListRecordsOldestParams{
		FromUs: from, ToUs: to, DoneFromUs: doneFrom, DoneToUs: doneTo, TypesCsv: typesCSV, OutcomesCsv: outcomesCSV, MaxRows: limit,
	}
	switch {
	case f.ByDone && f.Newest:
		rows, err = s.r.ListRecordsByDoneNewest(ctx, sqlitedb.ListRecordsByDoneNewestParams(args))
	case f.ByDone:
		rows, err = s.r.ListRecordsByDoneOldest(ctx, sqlitedb.ListRecordsByDoneOldestParams(args))
	case f.Newest:
		rows, err = s.r.ListRecordsNewest(ctx, sqlitedb.ListRecordsNewestParams(args))
	default:
		rows, err = s.r.ListRecordsOldest(ctx, args)
	}
	if err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}
	return recordsFromRows(rows)
}

func (s *Store) EachRecord(ctx context.Context, fn func(haccp.Record) error) error {
	const page = 500
	var after int64
	for {
		rows, err := s.r.RecordsAfterSeq(ctx, sqlitedb.RecordsAfterSeqParams{Seq: after, Limit: page})
		if err != nil {
			return fmt.Errorf("read records after %d: %w", after, err)
		}
		recs, err := recordsFromRows(rows)
		if err != nil {
			return err
		}
		for _, r := range recs {
			if err := fn(r); err != nil {
				return err
			}
			after = r.Seq
		}
		if len(rows) < page {
			return nil
		}
	}
}

// --- Limit versions ---

func (s *Store) AppendLimitVersion(ctx context.Context, lv haccp.LimitVersion) error {
	body, err := json.Marshal(lv)
	if err != nil {
		return fmt.Errorf("encode limit version: %w", err)
	}
	return mapErr(s.w.InsertLimitVersion(ctx, sqlitedb.InsertLimitVersionParams{
		ID:         lv.ID,
		CheckType:  string(lv.Check),
		ScopeKind:  string(lv.Scope.Kind),
		ScopeRef:   lv.Scope.Ref,
		Supersedes: nullStr(lv.Supersedes),
		CreatedBy:  lv.CreatedBy,
		CreatedAt:  us(lv.CreatedAt),
		Body:       string(body),
	}))
}

func decodeLimit(body string) (haccp.LimitVersion, error) {
	var lv haccp.LimitVersion
	if err := json.Unmarshal([]byte(body), &lv); err != nil {
		return lv, fmt.Errorf("decode limit version: %w", err)
	}
	lv.CreatedAt = lv.CreatedAt.UTC()
	return lv, nil
}

func (s *Store) GetLimitVersion(ctx context.Context, id string) (haccp.LimitVersion, error) {
	body, err := s.r.GetLimitVersion(ctx, id)
	if err != nil {
		return haccp.LimitVersion{}, mapErr(err)
	}
	return decodeLimit(body)
}

func (s *Store) ListLimitVersions(ctx context.Context) ([]haccp.LimitVersion, error) {
	bodies, err := s.r.ListLimitVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list limit versions: %w", err)
	}
	out := make([]haccp.LimitVersion, 0, len(bodies))
	for _, b := range bodies {
		lv, err := decodeLimit(b)
		if err != nil {
			return nil, err
		}
		out = append(out, lv)
	}
	return out, nil
}

// --- Users ---

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (s *Store) CreateUser(ctx context.Context, u auth.User) error {
	return mapErr(s.w.InsertUser(ctx, sqlitedb.InsertUserParams{
		ID:           u.ID,
		Name:         u.Name,
		Role:         string(u.Role),
		Active:       boolInt(u.Active),
		PinHash:      u.PINHash,
		PasswordHash: u.PasswordHash,
		CreatedAt:    us(u.CreatedAt),
		UpdatedAt:    us(u.UpdatedAt),
	}))
}

func (s *Store) UpdateUser(ctx context.Context, u auth.User) error {
	n, err := s.w.UpdateUser(ctx, sqlitedb.UpdateUserParams{
		Name:         u.Name,
		Role:         string(u.Role),
		Active:       boolInt(u.Active),
		PinHash:      u.PINHash,
		PasswordHash: u.PasswordHash,
		UpdatedAt:    us(u.UpdatedAt),
		AnonymisedAt: nullTime(u.AnonymisedAt),
		ID:           u.ID,
	})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func userFromRow(row sqlitedb.User) auth.User {
	return auth.User{
		ID:           row.ID,
		Name:         row.Name,
		Role:         auth.Role(row.Role),
		Active:       row.Active == 1,
		PINHash:      row.PinHash,
		PasswordHash: row.PasswordHash,
		CreatedAt:    fromUS(row.CreatedAt),
		UpdatedAt:    fromUS(row.UpdatedAt),
		AnonymisedAt: timePtr(row.AnonymisedAt),
	}
}

func (s *Store) GetUser(ctx context.Context, id string) (auth.User, error) {
	row, err := s.r.GetUser(ctx, id)
	if err != nil {
		return auth.User{}, mapErr(err)
	}
	return userFromRow(row), nil
}

func (s *Store) ListUsers(ctx context.Context) ([]auth.User, error) {
	rows, err := s.r.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	out := make([]auth.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, userFromRow(row))
	}
	return out, nil
}

// --- Sessions and API keys ---

func (s *Store) CreateSession(ctx context.Context, se auth.Session) error {
	return mapErr(s.w.InsertSession(ctx, sqlitedb.InsertSessionParams{
		TokenHash: se.TokenHash,
		UserID:    se.UserID,
		Method:    string(se.Method),
		CreatedAt: us(se.CreatedAt),
		ExpiresAt: us(se.ExpiresAt),
	}))
}

func (s *Store) GetSession(ctx context.Context, tokenHash string) (auth.Session, error) {
	row, err := s.r.GetSession(ctx, tokenHash)
	if err != nil {
		return auth.Session{}, mapErr(err)
	}
	return auth.Session{
		TokenHash: row.TokenHash,
		UserID:    row.UserID,
		Method:    auth.Method(row.Method),
		CreatedAt: fromUS(row.CreatedAt),
		ExpiresAt: fromUS(row.ExpiresAt),
	}, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	return mapErr(s.w.DeleteSession(ctx, tokenHash))
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	n, err := s.w.DeleteExpiredSessions(ctx, us(now))
	return n, mapErr(err)
}

func (s *Store) CreateAPIKey(ctx context.Context, k auth.APIKey) error {
	var revoked sql.NullInt64
	if k.RevokedAt != nil {
		revoked = sql.NullInt64{Int64: us(*k.RevokedAt), Valid: true}
	}
	return mapErr(s.w.InsertAPIKey(ctx, sqlitedb.InsertAPIKeyParams{
		ID:        k.ID,
		Kind:      string(k.Kind),
		UserID:    nullStr(k.UserID),
		Name:      k.Name,
		TokenHash: k.TokenHash,
		CreatedBy: k.CreatedBy,
		CreatedAt: us(k.CreatedAt),
		RevokedAt: revoked,
	}))
}

func keyFromRow(row sqlitedb.ApiKey) auth.APIKey {
	k := auth.APIKey{
		ID:        row.ID,
		Kind:      auth.KeyKind(row.Kind),
		UserID:    row.UserID.String,
		Name:      row.Name,
		TokenHash: row.TokenHash,
		CreatedBy: row.CreatedBy,
		CreatedAt: fromUS(row.CreatedAt),
	}
	if row.RevokedAt.Valid {
		t := fromUS(row.RevokedAt.Int64)
		k.RevokedAt = &t
	}
	return k
}

func (s *Store) GetAPIKeyByHash(ctx context.Context, tokenHash string) (auth.APIKey, error) {
	row, err := s.r.GetAPIKeyByHash(ctx, tokenHash)
	if err != nil {
		return auth.APIKey{}, mapErr(err)
	}
	return keyFromRow(row), nil
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]auth.APIKey, error) {
	rows, err := s.r.ListAPIKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list API keys: %w", err)
	}
	out := make([]auth.APIKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, keyFromRow(row))
	}
	return out, nil
}

func (s *Store) RevokeAPIKey(ctx context.Context, id string, at time.Time) error {
	n, err := s.w.RevokeAPIKey(ctx, sqlitedb.RevokeAPIKeyParams{
		RevokedAt: sql.NullInt64{Int64: us(at), Valid: true},
		ID:        id,
	})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// --- Files ---

func (s *Store) PutFile(ctx context.Context, m store.FileMeta) error {
	return mapErr(s.w.InsertFile(ctx, sqlitedb.InsertFileParams{
		Sha256:       m.SHA256,
		Mime:         m.MIME,
		Size:         m.Size,
		OriginalName: m.OriginalName,
		UploadedBy:   m.UploadedBy,
		UploadedAt:   us(m.UploadedAt),
	}))
}

func fileFromRow(row sqlitedb.File) store.FileMeta {
	return store.FileMeta{
		SHA256:       row.Sha256,
		MIME:         row.Mime,
		Size:         row.Size,
		OriginalName: row.OriginalName,
		UploadedBy:   row.UploadedBy,
		UploadedAt:   fromUS(row.UploadedAt),
	}
}

func (s *Store) GetFile(ctx context.Context, sha string) (store.FileMeta, error) {
	row, err := s.r.GetFile(ctx, sha)
	if err != nil {
		return store.FileMeta{}, mapErr(err)
	}
	return fileFromRow(row), nil
}

func (s *Store) ListFiles(ctx context.Context) ([]store.FileMeta, error) {
	rows, err := s.r.ListFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	out := make([]store.FileMeta, 0, len(rows))
	for _, row := range rows {
		out = append(out, fileFromRow(row))
	}
	return out, nil
}

// --- Settings ---

func (s *Store) AppendSettings(ctx context.Context, v store.SettingsVersion) error {
	return mapErr(s.w.InsertSettings(ctx, sqlitedb.InsertSettingsParams{
		ID:         v.ID,
		Kind:       string(v.Kind),
		Supersedes: nullStr(v.Supersedes),
		CreatedBy:  v.CreatedBy,
		CreatedAt:  us(v.CreatedAt),
		Reason:     v.Reason,
		PlanRef:    v.PlanRef,
		Body:       string(v.Body),
	}))
}

func settingsFromRow(row sqlitedb.SettingsVersion) store.SettingsVersion {
	return store.SettingsVersion{
		ID:         row.ID,
		Kind:       settings.Kind(row.Kind),
		Body:       []byte(row.Body),
		Supersedes: row.Supersedes.String,
		CreatedBy:  row.CreatedBy,
		CreatedAt:  fromUS(row.CreatedAt),
		Reason:     row.Reason,
		PlanRef:    row.PlanRef,
	}
}

func (s *Store) GetSettings(ctx context.Context, id string) (store.SettingsVersion, error) {
	row, err := s.r.GetSettings(ctx, id)
	if err != nil {
		return store.SettingsVersion{}, mapErr(err)
	}
	return settingsFromRow(row), nil
}

func (s *Store) ListSettings(ctx context.Context, kind settings.Kind) ([]store.SettingsVersion, error) {
	rows, err := s.r.ListSettings(ctx, string(kind))
	if err != nil {
		return nil, fmt.Errorf("list %s settings: %w", kind, err)
	}
	out := make([]store.SettingsVersion, 0, len(rows))
	for _, row := range rows {
		out = append(out, settingsFromRow(row))
	}
	return out, nil
}
