package crypto

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/GordenArcher/godenv"
)

// AES-256-GCM needs exactly this many key bytes, checked at Key() time so
// a misconfigured env var fails loudly at startup rather than producing
// a confusing cipher error deep inside Encrypt/Decrypt later.
const keyLengthBytes = 32

// KeyProvider is the seam between "how encryption works" and "where the
// key comes from". Everything in this package is written against this
// interface, not against EnvKeyProvider directly, so swapping in a real
// KMS (AWS KMS, GCP KMS, Vault) later is a new implementation of this
// interface, not a rewrite of Encryptor.
type KeyProvider interface {
	// Key returns the raw AES-256 key. Takes a context since a real KMS
	// call is a network round trip, an env var lookup just ignores it.
	Key(ctx context.Context) ([]byte, error)
}

// EnvKeyProvider is today's implementation, a base64-encoded key sitting
// in an environment variable. This is fine for local development and for
// a portfolio project, it is explicitly not fine for anything that will
// hold real people's biometric data, at that point this should be
// replaced with a KMS-backed KeyProvider, not extended with more env
// var special cases.
type EnvKeyProvider struct {
	EnvVar string
}

func (p EnvKeyProvider) Key(_ context.Context) ([]byte, error) {
	// Empty string fallback, godenv.Get never errors on a missing var,
	// the emptiness check right after is what actually catches that.
	raw := godenv.Get(p.EnvVar, "")
	if raw == "" {
		return nil, fmt.Errorf("environment variable %s is not set", p.EnvVar)
	}

	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode %s as base64: %w", p.EnvVar, err)
	}

	if len(key) != keyLengthBytes {
		return nil, fmt.Errorf("%s must decode to %d bytes for AES-256, got %d", p.EnvVar, keyLengthBytes, len(key))
	}

	return key, nil
}
