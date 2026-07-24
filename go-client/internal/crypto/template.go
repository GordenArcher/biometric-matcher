package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// Encryptor is the only thing storage code should talk to, it does not
// know or care whether the key came from an env var or a real KMS, that
// decision lives entirely behind KeyProvider.
type Encryptor struct {
	keys KeyProvider
}

func NewEncryptor(keys KeyProvider) *Encryptor {
	return &Encryptor{keys: keys}
}

// Encrypt returns nonce || ciphertext as a single blob, storing the nonce
// alongside the ciphertext (rather than in a separate column) keeps
// "one row, one encrypted value" true in the schema, at the cost of the
// caller never needing to think about nonces at all.
func (e *Encryptor) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	key, err := e.keys.Key(ctx)
	if err != nil {
		return nil, fmt.Errorf("get encryption key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("init aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// Seal appends the sealed ciphertext to its first argument, passing
	// nonce here means the result is nonce||ciphertext with no separate
	// concat step needed.
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return sealed, nil
}

// Decrypt expects exactly what Encrypt produced, nonce || ciphertext.
func (e *Encryptor) Decrypt(ctx context.Context, sealed []byte) ([]byte, error) {
	key, err := e.keys.Key(ctx)
	if err != nil {
		return nil, fmt.Errorf("get encryption key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("init aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return nil, fmt.Errorf("ciphertext shorter than nonce size, corrupt or wrong format")
	}

	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// GCM auth failure surfaces here, wrong key and tampered
		// ciphertext both land in this branch, deliberately not
		// distinguished since telling an attacker which one occurred
		// is information they shouldn't get.
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}
