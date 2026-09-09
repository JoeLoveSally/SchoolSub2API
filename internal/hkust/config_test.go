package hkust

import "testing"

func TestLoadConfigFromEnvDisabled(t *testing.T) {
	t.Setenv("HKUST_TOKEN", "")
	t.Setenv("HKUST_USE_API", "")
	t.Setenv("HKUST_WS_URL", "")
	t.Setenv("HKUST_ORIGIN", "")
	t.Setenv("HKUST_MODEL", "")

	_, enabled, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}
	if enabled {
		t.Fatal("LoadConfigFromEnv() enabled = true, want false")
	}
}

func TestLoadConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("HKUST_TOKEN", "token")
	t.Setenv("HKUST_USE_API", "use-api")
	t.Setenv("HKUST_WS_URL", "")
	t.Setenv("HKUST_ORIGIN", "")
	t.Setenv("HKUST_MODEL", "")

	cfg, enabled, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}
	if !enabled {
		t.Fatal("LoadConfigFromEnv() enabled = false, want true")
	}
	if cfg.Endpoint != defaultEndpoint {
		t.Fatalf("Endpoint = %q, want %q", cfg.Endpoint, defaultEndpoint)
	}
	if cfg.Origin != defaultOrigin {
		t.Fatalf("Origin = %q, want %q", cfg.Origin, defaultOrigin)
	}
	if cfg.Model != "GLM-5.2" {
		t.Fatalf("Model = %q, want GLM-5.2", cfg.Model)
	}
}

func TestLoadConfigFromEnvModelOverride(t *testing.T) {
	t.Setenv("HKUST_TOKEN", "token")
	t.Setenv("HKUST_USE_API", "use-api")
	t.Setenv("HKUST_WS_URL", "")
	t.Setenv("HKUST_ORIGIN", "")
	t.Setenv("HKUST_MODEL", "Kimi-K3")

	cfg, enabled, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}
	if !enabled {
		t.Fatal("LoadConfigFromEnv() enabled = false, want true")
	}
	if cfg.Model != "Kimi-K3" {
		t.Fatalf("Model = %q, want Kimi-K3", cfg.Model)
	}
}

func TestLoadConfigFromEnvRejectsPartialConfig(t *testing.T) {
	t.Setenv("HKUST_TOKEN", "token")
	t.Setenv("HKUST_USE_API", "")
	t.Setenv("HKUST_WS_URL", "")

	_, enabled, err := LoadConfigFromEnv()
	if !enabled {
		t.Fatal("LoadConfigFromEnv() enabled = false, want true for partial config")
	}
	if err == nil {
		t.Fatal("LoadConfigFromEnv() error = nil, want missing HKUST_USE_API error")
	}
}
