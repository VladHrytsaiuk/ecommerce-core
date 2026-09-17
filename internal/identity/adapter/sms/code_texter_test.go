package sms

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

type textSenderFake struct {
	phone, text string
	calls       int
}

func (f *textSenderFake) Send(_ context.Context, phone, text string) error {
	f.calls++
	f.phone, f.text = phone, text
	return nil
}

func TestACodeIsTextedFromTheTemplate(t *testing.T) {
	sender := &textSenderFake{}
	texter, err := NewCodeTexter(sender, "Код: {code}. Дійсний {minutes} хв.")
	if err != nil {
		t.Fatal(err)
	}
	if texter.Channel() != identity.CodeChannelPhone || texter.Transactional() {
		t.Fatal("a texter must be the phone channel and deliver outside the transaction")
	}
	message := identity.CodeMessage{ID: uuid.New(), Purpose: identity.CodePurposeSignIn, Destination: "+380501234567", Code: "012345", Lifetime: 90 * time.Second}
	if err := texter.SendCode(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if sender.phone != "+380501234567" || sender.text != "Код: 012345. Дійсний 2 хв." {
		t.Fatalf("sent (%q, %q)", sender.phone, sender.text)
	}
}

func TestTheTexterRefusesWhatItCannotSend(t *testing.T) {
	for name, template := range map[string]string{
		"no code":      "Your sign-in code is on its way",
		"two codes":    "{code} {code}",
		"far too long": "{code}" + strings.Repeat("x", 300),
	} {
		if _, err := NewCodeTexter(&textSenderFake{}, template); err == nil {
			t.Fatalf("%s: NewCodeTexter() accepted the template", name)
		}
	}
	if _, err := NewCodeTexter(nil, "{code}"); err == nil {
		t.Fatal("NewCodeTexter(nil) succeeded")
	}
	sender := &textSenderFake{}
	texter, _ := NewCodeTexter(sender, "{code}")
	valid := identity.CodeMessage{ID: uuid.New(), Purpose: identity.CodePurposeSignIn, Destination: "+380501234567", Code: "123456", Lifetime: time.Minute}
	for name, mutate := range map[string]func(*identity.CodeMessage){
		// Phone numbers are only for signing in; no reset or verification code
		// is texted.
		"a reset code": func(m *identity.CodeMessage) { m.Purpose = identity.CodePurposeResetPassword },
		"no number":    func(m *identity.CodeMessage) { m.Destination = "" },
		"no code":      func(m *identity.CodeMessage) { m.Code = "" },
		"no lifetime":  func(m *identity.CodeMessage) { m.Lifetime = 0 },
	} {
		message := valid
		mutate(&message)
		if err := texter.SendCode(context.Background(), message); err == nil {
			t.Fatalf("%s: SendCode() succeeded", name)
		}
	}
	if sender.calls != 0 {
		t.Fatal("an invalid message was texted")
	}
}
