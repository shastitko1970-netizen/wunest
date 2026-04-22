// Package byok ("bring-your-own-key") stores user-supplied provider API
// keys encrypted at rest and hands them back to the chat stream path when
// the user wants to bypass WuApi's managed pool. Uses AES-GCM with a
// process-wide 32-byte key from the SECRETS_KEY env var; each row carries
// its own random 12-byte nonce.
//
// Threat model: read-only DB access to nest_byok yields ciphertext + nonce
// only — without SECRETS_KEY, keys are useless. A compromised server with
// both DB and env does give the attacker everything; we accept that since
// no safer shape is feasible for a self-hosted service that needs to use
// the keys at request time (can't require user re-entry on every chat).
package byok

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// Encrypt seals plaintext under the given 32-byte key. Returns the
// ciphertext and its 12-byte nonce separately so the repo can store them
// in two columns — simpler to inspect than a concatenated blob.
func Encrypt(key []byte, plaintext string) (cipherText, nonce []byte, err error) {
	if len(key) != 32 {
		return nil, nil, fmt.Errorf("byok: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("byok: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("byok: new gcm: %w", err)
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("byok: random nonce: %w", err)
	}
	cipherText = gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return cipherText, nonce, nil
}

// Decrypt reverses Encrypt. Requires the exact nonce the row carried.
// Returns ErrInvalidCipher on any failure so handler layer can 500/404
// without leaking internal details.
func Decrypt(key, cipherText, nonce []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("byok: key must be 32 bytes, got %d", len(key))
	}
	if len(nonce) != 12 {
		return "", fmt.Errorf("byok: nonce must be 12 bytes, got %d", len(nonce))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("byok: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("byok: new gcm: %w", err)
	}
	plain, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return "", ErrInvalidCipher
	}
	return string(plain), nil
}

// ErrInvalidCipher is returned whenever AES-GCM fails to authenticate —
// wrong key, tampered row, wrong nonce. Handlers surface this as a 500
// rather than leaking the failure mode.
var ErrInvalidCipher = errors.New("byok: ciphertext authentication failed")
