package crypto

import (
	"bytes"
	"context"
	"testing"
)

// fixedKeyProvider lets tests hand the Encryptor a specific key without
// going through env vars, EnvKeyProvider itself is tested separately.
type fixedKeyProvider struct {
	key []byte
}

func (p fixedKeyProvider) Key(_ context.Context) ([]byte, error) {
	return p.key, nil
}

func mustKey(t *testing.T, b byte) []byte {
	t.Helper()
	key := make([]byte, keyLengthBytes)
	for i := range key {
		key[i] = b
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	enc := NewEncryptor(fixedKeyProvider{key: mustKey(t, 0x01)})
	plaintext := []byte("this stands in for a serialized SourceAFIS template")

	ciphertext, err := enc.Encrypt(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	// Sanity check this is actually doing something, not just echoing
	// the input back, a bug that returns plaintext unchanged would
	// otherwise pass every other assertion in this test.
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext equals plaintext, encryption did not happen")
	}

	got, err := enc.Decrypt(context.Background(), ciphertext)
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip mismatch, got %q, want %q", got, plaintext)
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	encryptor := NewEncryptor(fixedKeyProvider{key: mustKey(t, 0x01)})
	ciphertext, err := encryptor.Encrypt(context.Background(), []byte("secret template bytes"))
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	wrongKeyDecryptor := NewEncryptor(fixedKeyProvider{key: mustKey(t, 0x02)})
	if _, err := wrongKeyDecryptor.Decrypt(context.Background(), ciphertext); err == nil {
		t.Fatal("expected an error decrypting with the wrong key, got nil")
	}
}

func TestDecryptTamperedCiphertextFails(t *testing.T) {
	enc := NewEncryptor(fixedKeyProvider{key: mustKey(t, 0x01)})
	ciphertext, err := enc.Encrypt(context.Background(), []byte("secret template bytes"))
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	// Flip one bit past the nonce, GCM's auth tag should catch this,
	// this is the check that matters most for stored templates, a
	// corrupted row should never silently decrypt into garbage.
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[len(tampered)-1] ^= 0xFF

	if _, err := enc.Decrypt(context.Background(), tampered); err == nil {
		t.Fatal("expected an error decrypting tampered ciphertext, got nil")
	}
}

func TestDecryptRejectsShortInput(t *testing.T) {
	enc := NewEncryptor(fixedKeyProvider{key: mustKey(t, 0x01)})
	if _, err := enc.Decrypt(context.Background(), []byte("too short")); err == nil {
		t.Fatal("expected an error for input shorter than the nonce size, got nil")
	}
}
