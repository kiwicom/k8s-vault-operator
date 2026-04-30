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
