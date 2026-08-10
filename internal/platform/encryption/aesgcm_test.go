package encryption

import (
	"encoding/base64"
	"testing"
)

func TestAESGCMRoundTripAndWrongKeyRejection(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))
	cipher, err := NewAESGCM(key)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte(`{"email":"buyer@example.com"}`)
	ciphertext, err := cipher.Encrypt(plaintext)
	if err != nil || ciphertext == string(plaintext) {
		t.Fatalf("Encrypt() = %q, %v", ciphertext, err)
	}
	decrypted, err := cipher.Decrypt(ciphertext)
	if err != nil || string(decrypted) != string(plaintext) {
		t.Fatalf("Decrypt() = %q, %v", decrypted, err)
	}

	wrongKey := base64.StdEncoding.EncodeToString([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	wrongCipher, err := NewAESGCM(wrongKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrongCipher.Decrypt(ciphertext); err == nil {
		t.Fatal("Decrypt() with another key = nil error")
	}
}
