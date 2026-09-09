package main

import "testing"

func TestResolveBindHostDefaultsToAllInterfaces(t *testing.T) {
	t.Setenv("DS2API_BIND_HOST", "")
	if got := resolveBindHost(); got != "0.0.0.0" {
		t.Fatalf("resolveBindHost()=%q want %q", got, "0.0.0.0")
	}
}

func TestResolveBindHostUsesConfiguredLoopback(t *testing.T) {
	t.Setenv("DS2API_BIND_HOST", " 127.0.0.1 ")
	if got := resolveBindHost(); got != "127.0.0.1" {
		t.Fatalf("resolveBindHost()=%q want %q", got, "127.0.0.1")
	}
}
