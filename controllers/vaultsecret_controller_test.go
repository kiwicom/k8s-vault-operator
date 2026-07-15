package controllers

import (
	"strings"
	"testing"
)

func TestValidateVaultAddr(t *testing.T) {
	const defaultAddr = "https://vault.default.example.com"

	tests := []struct {
		name       string
		addr       string
		allowed    string
		wantErrSub string
	}{
		{
			name: "matches default addr",
			addr: defaultAddr,
		},
		{
			name:       "empty addr",
			addr:       "",
			wantErrSub: "invalid or empty vault address",
		},
		{
			name:       "malformed url",
			addr:       "::not a url::",
			wantErrSub: "invalid or empty vault address",
		},
		{
			name:       "missing scheme",
			addr:       "vault.example.com",
			wantErrSub: "invalid or empty vault address",
		},
		{
			name:       "custom addr rejected when allowlist empty",
			addr:       "https://vault.other.example.com",
			wantErrSub: "allowlist is empty",
		},
		{
			name:       "addr not in allowlist",
			addr:       "https://vault.other.example.com",
			allowed:    "https://vault.allowed.example.com",
			wantErrSub: "is not in the allowlist",
		},
		{
			name:    "addr in single-entry allowlist",
			addr:    "https://vault.allowed.example.com",
			allowed: "https://vault.allowed.example.com",
		},
		{
			name:    "addr in multi-entry allowlist with whitespace",
			addr:    "https://vault.b.example.com",
			allowed: "https://vault.a.example.com, https://vault.b.example.com ,https://vault.c.example.com",
		},
		{
			name:       "case-sensitive allowlist mismatch",
			addr:       "https://Vault.Allowed.Example.Com",
			allowed:    "https://vault.allowed.example.com",
			wantErrSub: "is not in the allowlist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVaultAddr(tt.addr, tt.allowed, defaultAddr)
			if tt.wantErrSub == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErrSub)
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErrSub, err.Error())
			}
		})
	}
}

// TestAuthSACacheKeyInjective guards against the cross-tenant secret disclosure
// caused by a non-injective cache key. Two tenants whose ("-"-joined) tuples
// collapse to the same string must still receive distinct cache keys, otherwise
// one tenant's reconcile can ride on the other's cached identity.
func TestAuthSACacheKeyInjective(t *testing.T) {
	const addr = "http://vault.vault.svc.cluster.local:8200"
	const role = "reader-prod"
	const authPath = "auth/kubernetes/login"

	// (namespace="team-a", saName="svc") vs (namespace="team", saName="a-svc")
	// both "-"-join to "...-team-a-svc-...".
	keyA := authSACacheKey(addr, "team-a", "svc", role, authPath)
	keyB := authSACacheKey(addr, "team", "a-svc", role, authPath)

	if keyA == keyB {
		t.Fatalf("cache key collision between distinct tenants: %q == %q", keyA, keyB)
	}

	// Same tuple must be stable (so caching actually works).
	if got := authSACacheKey(addr, "team-a", "svc", role, authPath); got != keyA {
		t.Fatalf("cache key not stable for identical input: %q != %q", got, keyA)
	}

	// A field boundary shift in any position must change the key.
	tuples := [][5]string{
		{addr, "ns", "sa", "role", "path"},
		{addr, "n", "ssa", "role", "path"},
		{addr, "ns", "sa", "rol", "epath"},
		{"http://a", "b", "sa", "role", "path"},
		{"http://ab", "", "sa", "role", "path"},
	}
	seen := map[string][5]string{}
	for _, tp := range tuples {
		k := authSACacheKey(tp[0], tp[1], tp[2], tp[3], tp[4])
		if prev, ok := seen[k]; ok {
			t.Fatalf("cache key collision: %v and %v both map to %q", prev, tp, k)
		}
		seen[k] = tp
	}
}
