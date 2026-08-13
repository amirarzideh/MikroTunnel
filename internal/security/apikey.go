package security

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/amirarzideh/MikroTunnel/internal/domain"
	"github.com/amirarzideh/MikroTunnel/internal/store"
)

const keyPrefix = "mt_"

func Create(ctx context.Context, db domain.TunnelStore) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := keyPrefix + base64.RawURLEncoding.EncodeToString(raw)
	prefix := secret[:11]
	sum := sha256.Sum256([]byte(secret))
	if err := db.CreateAPIKey(ctx, domain.APIKey{ID: store.NewID(), Prefix: prefix, Hash: hex.EncodeToString(sum[:]), CreatedAt: time.Now().Unix()}); err != nil {
		return "", err
	}
	return secret, nil
}

func Authenticate(ctx context.Context, db domain.TunnelStore, authorization string) error {
	const bearer = "Bearer "
	if !strings.HasPrefix(authorization, bearer) {
		return errors.New("missing bearer API key")
	}
	key := strings.TrimSpace(strings.TrimPrefix(authorization, bearer))
	if len(key) < 11 || !strings.HasPrefix(key, keyPrefix) {
		return errors.New("invalid API key")
	}
	stored, err := db.FindAPIKey(ctx, key[:11])
	if err != nil || stored.RevokedAt != nil {
		return errors.New("invalid API key")
	}
	sum := sha256.Sum256([]byte(key))
	actual := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(actual), []byte(stored.Hash)) != 1 {
		return errors.New("invalid API key")
	}
	return nil
}

func AuthenticateStatic(expected, authorization string) error {
	const bearer = "Bearer "
	if !strings.HasPrefix(authorization, bearer) {
		return errors.New("missing bearer API key")
	}
	actual := strings.TrimSpace(strings.TrimPrefix(authorization, bearer))
	if expected == "" || subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		return errors.New("invalid API key")
	}
	return nil
}

func AuthenticateDashboard(username, password, authorization string) error {
	const basic = "Basic "
	if !strings.HasPrefix(authorization, basic) {
		return errors.New("missing dashboard credentials")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(authorization, basic)))
	if err != nil {
		return errors.New("invalid dashboard credentials")
	}
	providedUser, providedPassword, ok := strings.Cut(string(raw), ":")
	if !ok || subtle.ConstantTimeCompare([]byte(providedUser), []byte(username)) != 1 || subtle.ConstantTimeCompare([]byte(providedPassword), []byte(password)) != 1 {
		return errors.New("invalid dashboard credentials")
	}
	return nil
}
