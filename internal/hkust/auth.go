package hkust

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"ds2api/internal/auth"
)

type APIKeyStore interface {
	HasAPIKey(string) bool
}

type Resolver struct {
	Store APIKeyStore
}

func NewResolver(store APIKeyStore) *Resolver {
	return &Resolver{Store: store}
}

func (r *Resolver) Determine(req *http.Request) (*auth.RequestAuth, error) {
	return r.determine(req)
}

func (r *Resolver) DetermineCaller(req *http.Request) (*auth.RequestAuth, error) {
	return r.determine(req)
}

func (r *Resolver) Release(_ *auth.RequestAuth) {}

func (r *Resolver) determine(req *http.Request) (*auth.RequestAuth, error) {
	key := extractAPIKey(req)
	if key == "" || r == nil || r.Store == nil || !r.Store.HasAPIKey(key) {
		return nil, auth.ErrUnauthorized
	}
	return &auth.RequestAuth{
		CallerID:      callerTokenID(key),
		TriedAccounts: map[string]bool{},
	}, nil
}

func extractAPIKey(req *http.Request) string {
	if req == nil {
		return ""
	}
	authHeader := strings.TrimSpace(req.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		if token := strings.TrimSpace(authHeader[7:]); token != "" {
			return token
		}
	}
	for _, header := range []string{"x-api-key", "x-goog-api-key"} {
		if key := strings.TrimSpace(req.Header.Get(header)); key != "" {
			return key
		}
	}
	if req.URL == nil {
		return ""
	}
	if key := strings.TrimSpace(req.URL.Query().Get("key")); key != "" {
		return key
	}
	return strings.TrimSpace(req.URL.Query().Get("api_key"))
}

func callerTokenID(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return "caller:" + hex.EncodeToString(sum[:8])
}
