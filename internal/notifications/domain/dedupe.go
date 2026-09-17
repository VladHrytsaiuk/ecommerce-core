package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

// DedupeKeyer builds the key that stops the same message being queued twice.
//
// It used to be the plain concatenation `type + ":" + recipient + ":" + payload`,
// which put the address and the rendered body in an indexed column — in the
// clear, and for every job including the ones whose payload was encrypted. A
// key is not a place to keep personal data.
//
// The replacement is an HMAC over the same three values, so it deduplicates
// exactly as before and reveals nothing. Its key is derived from the encryption
// key rather than being it: a leak of one must not be a forgery of the other,
// and deriving means no second secret for an operator to manage or rotate out
// of step.
type DedupeKeyer struct{ key []byte }

const dedupeKeyPurpose = "notifications:dedupe:v1"

func NewDedupeKeyer(encryptionKey string) (*DedupeKeyer, error) {
	if strings.TrimSpace(encryptionKey) == "" {
		return nil, fmt.Errorf("notification dedupe keyer requires the encryption key")
	}
	derived := make([]byte, sha256.Size)
	if _, err := io.ReadFull(hkdf.New(sha256.New, []byte(encryptionKey), nil, []byte(dedupeKeyPurpose)), derived); err != nil {
		return nil, fmt.Errorf("derive notification dedupe key: %w", err)
	}
	return &DedupeKeyer{key: derived}, nil
}

// Key is deterministic for the same message and carries none of it. The three
// values are length-prefixed rather than joined by a separator: a recipient
// containing the separator could otherwise collide with a different message.
func (k *DedupeKeyer) Key(messageType, recipient string, payload []byte) (string, error) {
	if k == nil || len(k.key) == 0 {
		return "", fmt.Errorf("notification dedupe keyer is not configured")
	}
	if strings.TrimSpace(messageType) == "" || strings.TrimSpace(recipient) == "" {
		return "", fmt.Errorf("notification dedupe key requires a type and a recipient")
	}
	canonical, err := json.Marshal([]any{messageType, strings.ToLower(strings.TrimSpace(recipient)), string(payload)})
	if err != nil {
		return "", fmt.Errorf("canonicalise notification dedupe input: %w", err)
	}
	mac := hmac.New(sha256.New, k.key)
	mac.Write(canonical)
	return hex.EncodeToString(mac.Sum(nil)), nil
}
