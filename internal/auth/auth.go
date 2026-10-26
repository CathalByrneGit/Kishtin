// Package auth holds users, roles, PIN and password hashing, API keys and
// sessions. It does no storage I/O.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Role is a staff role.
type Role string

const (
	RoleHeadChef     Role = "head_chef"
	RoleLineCook     Role = "line_cook"
	RoleFrontOfHouse Role = "front_of_house"
	RoleManager      Role = "manager"
)

// Roles lists every role.
var Roles = []Role{RoleHeadChef, RoleLineCook, RoleFrontOfHouse, RoleManager}

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	for _, k := range Roles {
		if r == k {
			return true
		}
	}
	return false
}

// User is a member of staff.
type User struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Role         Role      `json:"role"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	PINHash      string    `json:"-"`
	PasswordHash string    `json:"-"`
	// AnonymisedAt is when a leaver's name was replaced in the kitchen's
	// logs. Their HACCP records keep the name they were logged under.
	AnonymisedAt *time.Time `json:"anonymised_at,omitempty"`
}

// Method is how a principal proved who they are.
type Method string

const (
	MethodPIN      Method = "pin"
	MethodPassword Method = "password"
	// MethodKey is a user API key, used by the CLI and agents. It counts as
	// a full password login.
	MethodKey Method = "key"
)

// Session is a logged-in staff member on a device. Only the SHA-256 of the
// session token is stored.
type Session struct {
	TokenHash string    `json:"-"`
	UserID    string    `json:"user_id"`
	Method    Method    `json:"method"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Session lengths.
const (
	PINSessionTTL      = 12 * time.Hour
	PasswordSessionTTL = 30 * time.Minute
)

// KeyKind is the kind of API key.
type KeyKind string

const (
	// KeyDevice is a key for a shared kitchen device. It allows listing
	// staff and PIN login, and nothing else without a staff session.
	KeyDevice KeyKind = "device"
	// KeyUser acts as one user, for the CLI, scripts and agents.
	KeyUser KeyKind = "user"
)

// APIKey is an API key. Only the SHA-256 of the key is stored.
type APIKey struct {
	ID        string     `json:"id"`
	Kind      KeyKind    `json:"kind"`
	UserID    string     `json:"user_id,omitempty"`
	Name      string     `json:"name"`
	TokenHash string     `json:"-"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// Token prefixes make leaked tokens easy to recognise.
const (
	KeyPrefix     = "kishtin_key_"
	SessionPrefix = "kishtin_ses_"
)

// NewToken returns a random token with the given prefix and its hash.
func NewToken(prefix string) (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("read random bytes: %w", err)
	}
	token = prefix + base64.RawURLEncoding.EncodeToString(buf)
	return token, HashToken(token), nil
}

// HashToken returns the SHA-256 of a token as hex.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ValidatePIN checks a staff PIN: 4 to 8 digits.
func ValidatePIN(pin string) error {
	if len(pin) < 4 || len(pin) > 8 {
		return errors.New("a PIN is 4 to 8 digits")
	}
	for _, r := range pin {
		if r < '0' || r > '9' {
			return errors.New("a PIN is digits only")
		}
	}
	return nil
}

// ValidatePassword checks a manager password: at least 10 characters.
func ValidatePassword(pw string) error {
	if utf8.RuneCountInString(pw) < 10 {
		return errors.New("a password needs at least 10 characters")
	}
	return nil
}

// ValidateName checks a staff display name.
func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("a name is required")
	}
	if utf8.RuneCountInString(name) > 60 {
		return errors.New("a name is at most 60 characters")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("a name cannot contain control characters")
		}
	}
	return nil
}

// argon2id parameters, sized for a small kitchen mini-PC.
const (
	argonTime    = 2
	argonMemory  = 19 * 1024 // KiB
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashSecret hashes a PIN or password with argon2id.
func HashSecret(secret string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read random salt: %w", err)
	}
	key := argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifySecret reports whether secret matches an encoded argon2id hash.
func VerifySecret(encoded, secret string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var mem, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(secret), salt, t, mem, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
