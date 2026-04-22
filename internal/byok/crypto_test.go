package byok

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func randKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestEncrypt_RoundTrip(t *testing.T) {
	key := randKey(t)
	plain := "sk-totally-real-openai-key-1234"
	ct, nonce, err := Encrypt(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if len(nonce) != 12 {
		t.Errorf("nonce size: %d", len(nonce))
	}
	if bytes.Equal(ct, []byte(plain)) {
		t.Error("ciphertext should not equal plaintext")
	}
	got, err := Decrypt(key, ct, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Errorf("round-trip mismatch: %q", got)
	}
}

func TestEncrypt_RejectsWrongKeySize(t *testing.T) {
	if _, _, err := Encrypt(make([]byte, 16), "x"); err == nil {
		t.Error("16-byte key should be rejected")
	}
}

func TestDecrypt_WrongKeyReturnsErrInvalidCipher(t *testing.T) {
	ct, nonce, _ := Encrypt(randKey(t), "secret")
	_, err := Decrypt(randKey(t), ct, nonce)
	if err != ErrInvalidCipher {
		t.Errorf("expected ErrInvalidCipher, got %v", err)
	}
}

func TestDecrypt_TamperedCiphertextFails(t *testing.T) {
	key := randKey(t)
	ct, nonce, _ := Encrypt(key, "secret")
	ct[0] ^= 0xFF
	_, err := Decrypt(key, ct, nonce)
	if err != ErrInvalidCipher {
		t.Errorf("tampered ct should fail auth, got %v", err)
	}
}

func TestEncrypt_UniqueNoncesPerCall(t *testing.T) {
	key := randKey(t)
	_, n1, _ := Encrypt(key, "x")
	_, n2, _ := Encrypt(key, "x")
	if bytes.Equal(n1, n2) {
		t.Error("nonces should be unique per call")
	}
}
