package syncserver

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is returned by Authenticator.Login on bad
// username/password. It is deliberately generic (no user-enumeration
// signal).
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrInvalidToken is returned when a bearer token is missing or unknown.
var ErrInvalidToken = errors.New("invalid or expired token")

// Authenticator handles v1 auth: a single shared password, bcrypt-hashed
// at rest, with per-user rows kept for attribution and to leave room for
// per-user passwords later without a schema change.
type Authenticator struct {
	db *sql.DB
}

func NewAuthenticator(db *sql.DB) *Authenticator {
	return &Authenticator{db: db}
}

// EnsureSharedPassword makes sure every attempted username can authenticate
// against the single shared password by upserting a users row for it on
// first login (v1's "any username + the shared password" model), OR, if a
// canonical shared-password hash is already stored, verifies against that.
//
// canonicalHash is the bcrypt hash of the operator-configured shared
// password (from --password/--password-file), computed once at server
// startup.
func (a *Authenticator) Login(ctx context.Context, username, password, canonicalHash string) (string, error) {
	if username == "" || password == "" {
		return "", ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(canonicalHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	ts := now()
	if _, err := a.db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)
		ON CONFLICT(username) DO NOTHING`, username, canonicalHash, ts); err != nil {
		return "", fmt.Errorf("upsert user: %w", err)
	}

	token, err := newToken()
	if err != nil {
		return "", err
	}
	if _, err := a.db.ExecContext(ctx, `
		INSERT INTO sessions (token, username, created_at, last_seen_at) VALUES (?, ?, ?, ?)`,
		token, username, ts, ts); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

// Authorize resolves a bearer token to a username, bumping last_seen_at.
func (a *Authenticator) Authorize(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", ErrInvalidToken
	}
	var username string
	err := a.db.QueryRowContext(ctx, `SELECT username FROM sessions WHERE token = ?`, token).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", err
	}
	_, _ = a.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE token = ?`, now(), token)
	return username, nil
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashPassword bcrypt-hashes the operator-supplied shared password for
// storage/comparison. Never log the plaintext password.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(h), nil
}
