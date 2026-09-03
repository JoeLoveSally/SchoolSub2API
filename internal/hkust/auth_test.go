package hkust

import (
	"net/http/httptest"
	"testing"
)

type fakeAPIKeyStore map[string]bool

func (s fakeAPIKeyStore) HasAPIKey(key string) bool {
	return s[key]
}

func TestResolverAcceptsConfiguredAPIKey(t *testing.T) {
	resolver := NewResolver(fakeAPIKeyStore{"proxy-key": true})
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer proxy-key")

	a, err := resolver.Determine(req)
	if err != nil {
		t.Fatalf("Determine() error = %v", err)
	}
	if a.CallerID == "" {
		t.Fatal("Determine() CallerID is empty")
	}
	if a.DeepSeekToken != "" {
		t.Fatalf("Determine() DeepSeekToken = %q, want empty", a.DeepSeekToken)
	}
}

func TestResolverRejectsUnknownAPIKey(t *testing.T) {
	resolver := NewResolver(fakeAPIKeyStore{"proxy-key": true})
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")

	if _, err := resolver.Determine(req); err == nil {
		t.Fatal("Determine() error = nil, want unauthorized error")
	}
}
