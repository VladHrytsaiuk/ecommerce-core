package domain

import (
	"strings"
	"testing"
)

// The dedupe key used to be `type:recipient:payload` in an indexed column — the
// address and the rendered body in the clear, on every job, including the ones
// whose payload was encrypted.

func TestTheDedupeKeyRevealsNothingAboutTheMessage(t *testing.T) {
	keyer := mustKeyer(t, "an-encryption-key")

	key, err := keyer.Key("support_agent_reply", "customer@example.test", []byte(`{"message_body":"your refund is on the way"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"customer", "example.test", "refund", "support_agent_reply"} {
		if strings.Contains(key, secret) {
			t.Fatalf("the key contains %q: %s", secret, key)
		}
	}
}

func TestTheSameMessageAlwaysProducesTheSameKey(t *testing.T) {
	// It is the ON CONFLICT target, so it has to be stable across processes and
	// restarts for the same message.
	keyer := mustKeyer(t, "an-encryption-key")
	payload := []byte(`{"cart_id":"abc"}`)

	first, err := keyer.Key("abandoned_cart", "Buyer@Example.test", payload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := keyer.Key("abandoned_cart", "  buyer@example.test  ", payload)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("the same message produced two keys; the address is not normalised the way the writer normalises it")
	}
}

func TestDifferentMessagesProduceDifferentKeys(t *testing.T) {
	keyer := mustKeyer(t, "an-encryption-key")
	base, err := keyer.Key("abandoned_cart", "buyer@example.test", []byte(`{"step":1}`))
	if err != nil {
		t.Fatal(err)
	}
	for name, variant := range map[string]struct {
		messageType, recipient string
		payload                []byte
	}{
		"another type":      {"back_in_stock", "buyer@example.test", []byte(`{"step":1}`)},
		"another recipient": {"abandoned_cart", "other@example.test", []byte(`{"step":1}`)},
		"another payload":   {"abandoned_cart", "buyer@example.test", []byte(`{"step":2}`)},
	} {
		t.Run(name, func(t *testing.T) {
			other, err := keyer.Key(variant.messageType, variant.recipient, variant.payload)
			if err != nil {
				t.Fatal(err)
			}
			if other == base {
				t.Fatal("two different messages share a dedupe key; one of them would never be sent")
			}
		})
	}
}

func TestTheSeparatorCannotBeSmuggledAcrossFields(t *testing.T) {
	// The old key joined the three values with colons, so a recipient
	// containing one could be read as a different message entirely.
	keyer := mustKeyer(t, "an-encryption-key")
	first, err := keyer.Key("a", "b@x:c", []byte("d"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := keyer.Key("a", "b@x", []byte("c:d"))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("a value containing the separator collided with a different message")
	}
}

func TestTheDedupeKeyIsNotTheEncryptionKey(t *testing.T) {
	// A leak of one must not be a forgery of the other.
	keyer := mustKeyer(t, "an-encryption-key")
	if string(keyer.key) == "an-encryption-key" {
		t.Fatal("the encryption key is used directly for dedupe")
	}
	other := mustKeyer(t, "a-different-encryption-key")
	if string(keyer.key) == string(other.key) {
		t.Fatal("the derived key does not depend on the encryption key")
	}
}

func TestAKeyerIsRefusedWithoutASecret(t *testing.T) {
	if _, err := NewDedupeKeyer("   "); err == nil {
		t.Fatal("NewDedupeKeyer() accepted a blank key")
	}
}

func TestAnIncompleteMessageHasNoKey(t *testing.T) {
	keyer := mustKeyer(t, "an-encryption-key")
	for name, message := range map[string]struct{ messageType, recipient string }{
		"no type":      {"", "buyer@example.test"},
		"no recipient": {"abandoned_cart", "  "},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := keyer.Key(message.messageType, message.recipient, nil); err == nil {
				t.Fatal("Key() accepted a message it cannot identify")
			}
		})
	}
}

func mustKeyer(t *testing.T, secret string) *DedupeKeyer {
	t.Helper()
	keyer, err := NewDedupeKeyer(secret)
	if err != nil {
		t.Fatal(err)
	}
	return keyer
}
