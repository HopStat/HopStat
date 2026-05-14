package repo

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	plaintext := "my-secret-password"

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	if encrypted == "" {
		t.Error("encrypted should not be empty")
	}
	if encrypted == plaintext {
		t.Error("encrypted should differ from plaintext")
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestEncryptDifferentKey(t *testing.T) {
	key1 := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	key2 := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	encrypted, _ := Encrypt("test", key1)
	_, err := Decrypt(encrypted, key2)
	if err == nil {
		t.Error("expected error decrypting with wrong key")
	}
}

func TestEncryptInvalidKey(t *testing.T) {
	_, err := Encrypt("test", "not-hex")
	if err == nil {
		t.Error("expected error for invalid hex key")
	}
}

func TestEncryptShortKey(t *testing.T) {
	_, err := Encrypt("test", "abcdef")
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestDecryptInvalidCiphertext(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	_, err := Decrypt("abcd", key)
	if err == nil {
		t.Error("expected error for short ciphertext")
	}
}

func TestEncryptEmptyPlaintext(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	encrypted, err := Encrypt("", key)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}
	if decrypted != "" {
		t.Errorf("expected empty, got %q", decrypted)
	}
}
