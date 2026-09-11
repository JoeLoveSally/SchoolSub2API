package shared

import "testing"

func TestEmptyOutputRetryDisabledForHKUST(t *testing.T) {
	t.Setenv("HKUST_TOKEN", "test-token")
	t.Setenv("HKUST_USE_API", "test-use-api")
	if EmptyOutputRetryEnabled() {
		t.Fatal("expected synthetic empty-output retry to be disabled in HKUST mode")
	}
}
