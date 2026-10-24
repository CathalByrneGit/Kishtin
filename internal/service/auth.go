package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cathalbyrnegit/kishtin/internal/auth"
	"github.com/cathalbyrnegit/kishtin/internal/clock"
	"github.com/cathalbyrnegit/kishtin/internal/events"
	"github.com/cathalbyrnegit/kishtin/internal/store"
)

// Caller is who is making a request: the API key used and, if known, the
// staff member.
type Caller struct {
	Key       auth.APIKey
	Principal *auth.Principal
}

// Authenticate checks an API key and, for device keys, an optional staff
// session token.
func (s *Service) Authenticate(ctx context.Context, apiKey, sessionToken string) (Caller, error) {
	if apiKey == "" {
		return Caller{}, newErr(KindUnauthenticated, "an API key is required")
	}
	key, err := s.st.Auth().GetAPIKeyByHash(ctx, auth.HashToken(apiKey))
	if errors.Is(err, store.ErrNotFound) || (err == nil && key.RevokedAt != nil) {
		return Caller{}, newErr(KindUnauthenticated, "this API key is not valid")
	}
	if err != nil {
		return Caller{}, fmt.Errorf("look up API key: %w", err)
	}
	c := Caller{Key: key}

	switch key.Kind {
	case auth.KeyUser:
		u, err := s.st.Users().GetUser(ctx, key.UserID)
		if err != nil {
			return Caller{}, fmt.Errorf("load key owner: %w", err)
		}
		if !u.Active {
			return Caller{}, newErr(KindUnauthenticated, "the owner of this API key is deactivated")
		}
		c.Principal = &auth.Principal{User: u, Method: auth.MethodKey}
	case auth.KeyDevice:
		if sessionToken == "" {
			return c, nil
		}
		se, err := s.st.Auth().GetSession(ctx, auth.HashToken(sessionToken))
		if errors.Is(err, store.ErrNotFound) {
			return Caller{}, newErr(KindUnauthenticated, "your session has ended: log in again")
		}
		if err != nil {
			return Caller{}, fmt.Errorf("look up session: %w", err)
		}
		if !s.clk.Now().Before(se.ExpiresAt) {
			return Caller{}, newErr(KindUnauthenticated, "your session has ended: log in again")
		}
		u, err := s.st.Users().GetUser(ctx, se.UserID)
		if err != nil {
			return Caller{}, fmt.Errorf("load session user: %w", err)
		}
		if !u.Active {
			return Caller{}, newErr(KindUnauthenticated, "this user is deactivated")
		}
		c.Principal = &auth.Principal{User: u, Method: se.Method}
	default:
		return Caller{}, newErr(KindUnauthenticated, "this API key is not valid")
	}
	return c, nil
}

// StaffEntry is what the PIN login screen shows for each active user.
type StaffEntry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Role        auth.Role `json:"role"`
	HasPIN      bool      `json:"has_pin"`
	HasPassword bool      `json:"has_password"`
}

// StaffList lists active staff for the login screen. Any valid key can
// call it.
func (s *Service) StaffList(ctx context.Context) ([]StaffEntry, error) {
	users, err := s.st.Users().ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := []StaffEntry{}
	for _, u := range users {
		if u.Active {
			out = append(out, StaffEntry{ID: u.ID, Name: u.Name, Role: u.Role, HasPIN: u.PINHash != "", HasPassword: u.PasswordHash != ""})
		}
	}
	return out, nil
}

// LoginResult is a new staff session.
type LoginResult struct {
	Token   string       `json:"token"`
	Session auth.Session `json:"session"`
	User    auth.User    `json:"user"`
}

// LoginPIN starts a PIN session on a device.
func (s *Service) LoginPIN(ctx context.Context, c Caller, userID, pin string) (LoginResult, error) {
	return s.login(ctx, c, userID, pin, auth.MethodPIN)
}

// LoginPassword starts a short manager password session on a device.
func (s *Service) LoginPassword(ctx context.Context, c Caller, userID, password string) (LoginResult, error) {
	return s.login(ctx, c, userID, password, auth.MethodPassword)
}

func (s *Service) login(ctx context.Context, c Caller, userID, secret string, m auth.Method) (LoginResult, error) {
	if c.Key.Kind != auth.KeyDevice {
		return LoginResult{}, newErr(KindForbidden, "staff log in on a kitchen device; this key is not a device key")
	}
	now := s.clk.Now()
	if locked, until := s.lockout.Locked(userID, now); locked {
		return LoginResult{}, newErr(KindLocked, "too many wrong tries: try again after %s", until.In(clock.Dublin).Format("15:04"))
	}
	u, err := s.st.Users().GetUser(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return LoginResult{}, newErr(KindUnauthenticated, "wrong name or %s", m)
	}
	if err != nil {
		return LoginResult{}, err
	}
	hash := u.PINHash
	if m == auth.MethodPassword {
		hash = u.PasswordHash
	}
	if !u.Active || hash == "" || !auth.VerifySecret(hash, secret) {
		s.lockout.Fail(userID, now)
		return LoginResult{}, newErr(KindUnauthenticated, "wrong name or %s", m)
	}
	s.lockout.Succeed(userID)

	token, tokenHash, err := auth.NewToken(auth.SessionPrefix)
	if err != nil {
		return LoginResult{}, err
	}
	ttl := auth.PINSessionTTL
	if m == auth.MethodPassword {
		ttl = auth.PasswordSessionTTL
	}
	se := auth.Session{TokenHash: tokenHash, UserID: u.ID, Method: m, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	if err := s.st.Auth().CreateSession(ctx, se); err != nil {
		return LoginResult{}, fmt.Errorf("save session: %w", err)
	}
	return LoginResult{Token: token, Session: se, User: u}, nil
}

// Logout ends a session. Ending a session that doesn't exist is not an
// error.
func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	if sessionToken == "" {
		return nil
	}
	return s.st.Auth().DeleteSession(ctx, auth.HashToken(sessionToken))
}

// CleanSessions removes expired sessions.
func (s *Service) CleanSessions(ctx context.Context) (int64, error) {
	return s.st.Auth().DeleteExpiredSessions(ctx, s.clk.Now())
}

// NewUser is a staff member to add.
type NewUser struct {
	Name     string    `json:"name"`
	Role     auth.Role `json:"role"`
	PIN      string    `json:"pin,omitempty"`
	Password string    `json:"password,omitempty"`
}

func (s *Service) buildUser(nu NewUser) (auth.User, error) {
	nu.Name = strings.TrimSpace(nu.Name)
	if err := auth.ValidateName(nu.Name); err != nil {
		return auth.User{}, newErr(KindInvalid, "%s", err)
	}
	if !nu.Role.Valid() {
		return auth.User{}, newErr(KindInvalid, "unknown role %q", nu.Role)
	}
	if nu.PIN == "" && nu.Password == "" {
		return auth.User{}, newErr(KindInvalid, "set a PIN, a password or both")
	}
	if nu.Role == auth.RoleManager && nu.Password == "" {
		return auth.User{}, newErr(KindInvalid, "a manager needs a password")
	}
	now := s.clk.Now()
	u := auth.User{ID: newID(), Name: nu.Name, Role: nu.Role, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := setSecrets(&u, nu.PIN, nu.Password); err != nil {
		return auth.User{}, err
	}
	return u, nil
}

func setSecrets(u *auth.User, pin, password string) error {
	if pin != "" {
		if err := auth.ValidatePIN(pin); err != nil {
			return newErr(KindInvalid, "%s", err)
		}
		h, err := auth.HashSecret(pin)
		if err != nil {
			return err
		}
		u.PINHash = h
	}
	if password != "" {
		if err := auth.ValidatePassword(password); err != nil {
			return newErr(KindInvalid, "%s", err)
		}
		h, err := auth.HashSecret(password)
		if err != nil {
			return err
		}
		u.PasswordHash = h
	}
	return nil
}

// SetupResult is the first manager and their API key.
type SetupResult struct {
	User   auth.User   `json:"user"`
	Key    auth.APIKey `json:"key"`
	Secret string      `json:"secret"`
}

// Setup creates the first manager and a user API key for them. It only
// works while there are no users.
func (s *Service) Setup(ctx context.Context, nu NewUser) (SetupResult, error) {
	nu.Role = auth.RoleManager
	u, err := s.buildUser(nu)
	if err != nil {
		return SetupResult{}, err
	}
	var res SetupResult
	err = s.st.WithTx(ctx, func(tx store.Store) error {
		existing, err := tx.Users().ListUsers(ctx)
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return newErr(KindConflict, "setup has already been done: add staff from a manager account")
		}
		if err := tx.Users().CreateUser(ctx, u); err != nil {
			return mapStoreErr(err, "user")
		}
		key, secret, err := s.newKey(ctx, tx, auth.KeyUser, "Setup key for "+u.Name, u.ID, u.ID)
		if err != nil {
			return err
		}
		res = SetupResult{User: u, Key: key, Secret: secret}
		return nil
	})
	return res, err
}

// CreateUser adds a staff member.
func (s *Service) CreateUser(ctx context.Context, p *auth.Principal, nu NewUser) (auth.User, error) {
	if err := allow(p, auth.ActManageUsers); err != nil {
		return auth.User{}, err
	}
	u, err := s.buildUser(nu)
	if err != nil {
		return auth.User{}, err
	}
	if err := s.st.Users().CreateUser(ctx, u); err != nil {
		return auth.User{}, mapStoreErr(err, "a user with that name")
	}
	s.publish(events.KindStaff, u.ID)
	return u, nil
}

// UserChange is a change to a staff member. Nil fields are left as they
// are.
type UserChange struct {
	Name     *string    `json:"name,omitempty"`
	Role     *auth.Role `json:"role,omitempty"`
	Active   *bool      `json:"active,omitempty"`
	PIN      *string    `json:"pin,omitempty"`
	Password *string    `json:"password,omitempty"`
}

// UpdateUser changes a staff member.
func (s *Service) UpdateUser(ctx context.Context, p *auth.Principal, id string, ch UserChange) (auth.User, error) {
	if err := allow(p, auth.ActManageUsers); err != nil {
		return auth.User{}, err
	}
	var out auth.User
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		u, err := tx.Users().GetUser(ctx, id)
		if err != nil {
			return mapStoreErr(err, "user")
		}
		if ch.Name != nil {
			name := strings.TrimSpace(*ch.Name)
			if err := auth.ValidateName(name); err != nil {
				return newErr(KindInvalid, "%s", err)
			}
			u.Name = name
		}
		if ch.Role != nil {
			if !ch.Role.Valid() {
				return newErr(KindInvalid, "unknown role %q", *ch.Role)
			}
			u.Role = *ch.Role
		}
		if ch.Active != nil {
			u.Active = *ch.Active
		}
		var pin, pw string
		if ch.PIN != nil {
			pin = *ch.PIN
		}
		if ch.Password != nil {
			pw = *ch.Password
		}
		if err := setSecrets(&u, pin, pw); err != nil {
			return err
		}
		if u.Role == auth.RoleManager && u.PasswordHash == "" {
			return newErr(KindInvalid, "a manager needs a password")
		}
		if u.ID == p.User.ID && (!u.Active || u.Role != auth.RoleManager) {
			return newErr(KindInvalid, "you cannot remove your own manager access")
		}
		u.UpdatedAt = s.clk.Now()
		if err := tx.Users().UpdateUser(ctx, u); err != nil {
			return mapStoreErr(err, "a user with that name")
		}
		out = u
		return nil
	})
	if err == nil {
		s.publish(events.KindStaff, out.ID)
	}
	return out, err
}

// ListUsers lists every staff member, including deactivated ones.
func (s *Service) ListUsers(ctx context.Context, p *auth.Principal) ([]auth.User, error) {
	if err := allow(p, auth.ActManageUsers); err != nil {
		return nil, err
	}
	return s.st.Users().ListUsers(ctx)
}

// NewKeyResult is a new API key. Secret is shown once and never stored.
type NewKeyResult struct {
	Key    auth.APIKey `json:"key"`
	Secret string      `json:"secret"`
}

// CreateKey creates a device key or a user key. A user key for another
// user needs that user to exist and be active.
func (s *Service) CreateKey(ctx context.Context, p *auth.Principal, kind auth.KeyKind, name, userID string) (NewKeyResult, error) {
	if err := allow(p, auth.ActManageKeys); err != nil {
		return NewKeyResult{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return NewKeyResult{}, newErr(KindInvalid, "give the key a name, such as \"Pass tablet\"")
	}
	var res NewKeyResult
	err := s.st.WithTx(ctx, func(tx store.Store) error {
		switch kind {
		case auth.KeyDevice:
			userID = ""
		case auth.KeyUser:
			if userID == "" {
				userID = p.User.ID
			}
			u, err := tx.Users().GetUser(ctx, userID)
			if err != nil {
				return mapStoreErr(err, "user")
			}
			if !u.Active {
				return newErr(KindInvalid, "%s is deactivated", u.Name)
			}
		default:
			return newErr(KindInvalid, "key kind must be device or user")
		}
		key, secret, err := s.newKey(ctx, tx, kind, name, userID, p.User.ID)
		if err != nil {
			return err
		}
		res = NewKeyResult{Key: key, Secret: secret}
		return nil
	})
	return res, err
}

func (s *Service) newKey(ctx context.Context, tx store.Store, kind auth.KeyKind, name, userID, createdBy string) (auth.APIKey, string, error) {
	secret, hash, err := auth.NewToken(auth.KeyPrefix)
	if err != nil {
		return auth.APIKey{}, "", err
	}
	k := auth.APIKey{ID: newID(), Kind: kind, UserID: userID, Name: name, TokenHash: hash, CreatedBy: createdBy, CreatedAt: s.clk.Now()}
	if err := tx.Auth().CreateAPIKey(ctx, k); err != nil {
		return auth.APIKey{}, "", fmt.Errorf("save API key: %w", err)
	}
	return k, secret, nil
}

// ListKeys lists API keys, including revoked ones.
func (s *Service) ListKeys(ctx context.Context, p *auth.Principal) ([]auth.APIKey, error) {
	if err := allow(p, auth.ActManageKeys); err != nil {
		return nil, err
	}
	return s.st.Auth().ListAPIKeys(ctx)
}

// RevokeKey revokes an API key.
func (s *Service) RevokeKey(ctx context.Context, p *auth.Principal, id string) error {
	if err := allow(p, auth.ActManageKeys); err != nil {
		return err
	}
	if err := s.st.Auth().RevokeAPIKey(ctx, id, s.clk.Now()); err != nil {
		return mapStoreErr(err, "active API key")
	}
	return nil
}
