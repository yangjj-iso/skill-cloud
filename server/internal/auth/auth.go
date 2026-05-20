// Package auth provides multi-tenant identity, API key management, and
// the Bearer-token middleware used by the v1 API.
//
// API keys are issued as `<prefix>.<secret>` strings where:
//
//   - `prefix` is a short, non-secret identifier (e.g. `sc_live_a1b2c3`) that
//     uniquely indexes the row in the database.
//   - `secret` is a high-entropy random string. Only its bcrypt hash is
//     stored; the raw value is shown to the user exactly once at creation
//     time and never persisted.
//
// Validation works in two steps:
//
//  1. Look up the row by `prefix` (cheap, indexed).
//  2. Compare the presented secret to the stored bcrypt hash.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Principal is the authenticated identity for a request.
type Principal struct {
	OrgID    uuid.UUID
	UserID   uuid.UUID
	APIKeyID uuid.UUID
}

// IssuedKey is returned exactly once when an API key is created.
type IssuedKey struct {
	ID     uuid.UUID
	Prefix string
	// Plaintext is the full `<prefix>.<secret>` string. It is shown to the
	// user once and never persisted.
	Plaintext string
}

// ErrInvalidKey is returned when a presented API key cannot be authenticated.
var ErrInvalidKey = errors.New("invalid api key")

// Service issues and validates API keys.
type Service struct {
	pool *pgxpool.Pool
}

// NewService constructs an auth service backed by the given pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// CreateOrg inserts a new organization.
func (s *Service) CreateOrg(ctx context.Context, slug, name string) (uuid.UUID, error) {
	id := uuid.New()
	_, err := s.pool.Exec(ctx, `INSERT INTO orgs (id, slug, name) VALUES ($1, $2, $3)`, id, slug, name)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert org: %w", err)
	}
	return id, nil
}

// CreateUser inserts a new user and adds them as a member of orgID.
func (s *Service) CreateUser(ctx context.Context, orgID uuid.UUID, email string) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	userID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1, $2)`, userID, email); err != nil {
		return uuid.Nil, fmt.Errorf("insert user: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, 'owner')`, orgID, userID); err != nil {
		return uuid.Nil, fmt.Errorf("insert member: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit: %w", err)
	}
	return userID, nil
}

// IssueAPIKey creates a new API key for the (orgID, userID) pair and returns
// the plaintext key, which must be shown to the caller exactly once.
func (s *Service) IssueAPIKey(ctx context.Context, orgID, userID uuid.UUID, name string) (IssuedKey, error) {
	prefix, err := randomToken(6)
	if err != nil {
		return IssuedKey{}, fmt.Errorf("generate prefix: %w", err)
	}
	prefix = "sc_live_" + prefix
	secret, err := randomToken(24)
	if err != nil {
		return IssuedKey{}, fmt.Errorf("generate secret: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return IssuedKey{}, fmt.Errorf("hash secret: %w", err)
	}

	id := uuid.New()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO api_keys (id, org_id, user_id, prefix, hash, name)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, orgID, userID, prefix, string(hash), name)
	if err != nil {
		return IssuedKey{}, fmt.Errorf("insert api key: %w", err)
	}

	return IssuedKey{ID: id, Prefix: prefix, Plaintext: prefix + "." + secret}, nil
}

// LookupOrgSlug returns the slug for the given orgID. Used by metrics
// to label series with a human-friendly identifier instead of a UUID.
// Returns (slug, nil) on success or ("", error) on failure.
func (s *Service) LookupOrgSlug(ctx context.Context, orgID uuid.UUID) (string, error) {
	var slug string
	err := s.pool.QueryRow(ctx, `SELECT slug FROM orgs WHERE id = $1`, orgID).Scan(&slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("lookup org slug: %w", err)
	}
	return slug, nil
}

// Authenticate resolves a presented `<prefix>.<secret>` token to a Principal.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	prefix, secret, ok := strings.Cut(token, ".")
	if !ok || prefix == "" || secret == "" {
		return Principal{}, ErrInvalidKey
	}

	var (
		row        Principal
		storedHash string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, user_id, hash
		FROM api_keys
		WHERE prefix = $1
	`, prefix).Scan(&row.APIKeyID, &row.OrgID, &row.UserID, &storedHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, ErrInvalidKey
		}
		return Principal{}, fmt.Errorf("lookup api key: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(secret)); err != nil {
		return Principal{}, ErrInvalidKey
	}

	// Best-effort last_used update; failures here don't block the request.
	_, _ = s.pool.Exec(ctx, `UPDATE api_keys SET last_used_at = now() WHERE id = $1`, row.APIKeyID)
	return row, nil
}

func randomToken(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
