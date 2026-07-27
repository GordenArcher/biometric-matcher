package crypto

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestEnvKeyProviderMissingVar(t *testing.T) {
	// Deliberately not set, t.Setenv isn't used here, an unset var is
	// the case under test.
	p := EnvKeyProvider{EnvVar: "BIOMETRIC_MATCHER_TEST_KEY_NOT_SET"}
	if _, err := p.Key(context.Background()); err == nil {
		t.Fatal("expected an error for an unset env var, got nil")
	}
}

func TestEnvKeyProviderInvalidBase64(t *testing.T) {
	t.Setenv("BIOMETRIC_MATCHER_TEST_KEY", "not valid base64!!!")
	p := EnvKeyProvider{EnvVar: "BIOMETRIC_MATCHER_TEST_KEY"}
	if _, err := p.Key(context.Background()); err == nil {
		t.Fatal("expected an error for invalid base64, got nil")
	}
}

func TestEnvKeyProviderWrongLength(t *testing.T) {
	// Valid base64, wrong decoded length, 16 bytes would be a fine
	// AES-128 key but this provider is specifically for AES-256.
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	t.Setenv("BIOMETRIC_MATCHER_TEST_KEY", short)
	p := EnvKeyProvider{EnvVar: "BIOMETRIC_MATCHER_TEST_KEY"}
	if _, err := p.Key(context.Background()); err == nil {
		t.Fatal("expected an error for a 16-byte key, got nil")
	}
}

func TestEnvKeyProviderValid(t *testing.T) {
	want := make([]byte, keyLengthBytes)
	for i := range want {
		want[i] = byte(i)
	}
	t.Setenv("BIOMETRIC_MATCHER_TEST_KEY", base64.StdEncoding.EncodeToString(want))

	p := EnvKeyProvider{EnvVar: "BIOMETRIC_MATCHER_TEST_KEY"}
	got, err := p.Key(context.Background())
	if err != nil {
		t.Fatalf("Key returned error: %v", err)
	}
	if len(got) != keyLengthBytes {
		t.Fatalf("got key of length %d, want %d", len(got), keyLengthBytes)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("key byte %d mismatch, got %d, want %d", i, got[i], want[i])
		}
	}
}
