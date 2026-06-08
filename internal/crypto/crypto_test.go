package crypto

import (
	"encoding/hex"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := 0; i < 32; i++ {
		key[i] = byte(i)
	}
	keyHex := hex.EncodeToString(key)

	e, err := NewEncrypter(keyHex)
	if err != nil {
		t.Fatalf("NewEncrypter failed: %v", err)
	}

	plaintext := "hello world"
	encrypted, err := e.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := e.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestEncryptProducesDifferentOutput(t *testing.T) {
	key := make([]byte, 32)
	for i := 0; i < 32; i++ {
		key[i] = byte(i)
	}
	keyHex := hex.EncodeToString(key)

	e, err := NewEncrypter(keyHex)
	if err != nil {
		t.Fatalf("NewEncrypter failed: %v", err)
	}

	plaintext := "test string"
	encrypted1, err := e.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("first Encrypt failed: %v", err)
	}

	encrypted2, err := e.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("second Encrypt failed: %v", err)
	}

	if encrypted1 == encrypted2 {
		t.Error("expected different ciphertexts due to random nonce, but got identical outputs")
	}
}

func TestDecryptInvalidInput(t *testing.T) {
	key := make([]byte, 32)
	for i := 0; i < 32; i++ {
		key[i] = byte(i)
	}
	keyHex := hex.EncodeToString(key)

	e, err := NewEncrypter(keyHex)
	if err != nil {
		t.Fatalf("NewEncrypter failed: %v", err)
	}

	_, err = e.Decrypt("invalid-garbage-base64!!!")
	if err == nil {
		t.Error("expected Decrypt to fail on invalid input, but it succeeded")
	}
}

func TestNewEncrypterInvalidKey(t *testing.T) {
	tests := []struct {
		name    string
		keyHex  string
		wantErr bool
	}{
		{"invalid hex", "not-valid-hex", true},
		{"too short", "0102030405060708090a0b0c0d0e0f10", true}, // 16 bytes, need 32
		{"too long", "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f2021", true}, // 33 bytes
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewEncrypter(tt.keyHex)
			if (err == nil) != !tt.wantErr {
				t.Errorf("NewEncrypter got error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
