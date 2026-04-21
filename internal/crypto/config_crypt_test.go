package crypto_test

import (
	"testing"

	configcrypto "github.com/Tencent/WeKnora/internal/crypto"
)

const testKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

func TestEncryptDecryptRoundtrip(t *testing.T) {
	plaintext := []byte(`{"credentials":{"api_key":"secret-value"}}`)
	ciphertext, err := configcrypto.Encrypt(plaintext, testKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if ciphertext == string(plaintext) {
		t.Fatal("ciphertext should differ from plaintext")
	}
	decrypted, err := configcrypto.Decrypt(ciphertext, testKey)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptProducesDistinctCiphertexts(t *testing.T) {
	pt := []byte("same plaintext")
	c1, _ := configcrypto.Encrypt(pt, testKey)
	c2, _ := configcrypto.Encrypt(pt, testKey)
	if c1 == c2 {
		t.Fatal("two encryptions of same plaintext should differ")
	}
}

func TestDecryptInvalidKeyReturnsError(t *testing.T) {
	if _, err := configcrypto.Decrypt("anyciphertext", "tooshort"); err == nil {
		t.Fatal("expected error for invalid key")
	}
}
