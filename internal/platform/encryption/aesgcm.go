// Package encryption provides small, auditable authenticated encryption
// primitives for infrastructure adapters. Callers own key rotation policy.
package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// AESGCM stores a random nonce alongside ciphertext in standard base64.
// Keys must be base64-encoded 256-bit values generated with
// `openssl rand -base64 32`.
type AESGCM struct{ aead cipher.AEAD }

func NewAESGCM(encodedKey string) (*AESGCM, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("notification encryption key must be base64-encoded 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AES-GCM: %w", err)
	}
	return &AESGCM{aead: aead}, nil
}

func (c *AESGCM) Encrypt(plaintext []byte) (string, error) {
	if c == nil || c.aead == nil {
		return "", fmt.Errorf("AES-GCM is not configured")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read encryption nonce: %w", err)
	}
	ciphertext := c.aead.Seal(nil, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func (c *AESGCM) Decrypt(encoded string) ([]byte, error) {
	if c == nil || c.aead == nil {
		return nil, fmt.Errorf("AES-GCM is not configured")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) < c.aead.NonceSize() {
		return nil, fmt.Errorf("invalid encrypted payload")
	}
	nonce, ciphertext := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt payload: %w", err)
	}
	return plaintext, nil
}
