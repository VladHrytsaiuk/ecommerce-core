package notifications

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
)

type schedulerFake struct {
	jobType, locale, email string
	payload                any
	calls                  int
}

func (f *schedulerFake) ScheduleEmail(_ context.Context, jobType, locale, email string, payload any) error {
	f.calls++
	f.jobType, f.locale, f.email, f.payload = jobType, locale, email, payload
	return nil
}

func TestTheMailerQueuesEachPurposeWithItsOwnTemplate(t *testing.T) {
	for purpose, template := range map[identity.CodePurpose]string{
		identity.CodePurposeSignIn:        notifications.SignInCodeTemplate,
		identity.CodePurposeVerifyEmail:   notifications.EmailVerificationCodeTemplate,
		identity.CodePurposeResetPassword: notifications.PasswordResetCodeTemplate,
	} {
		t.Run(string(purpose), func(t *testing.T) {
			scheduler := &schedulerFake{}
			mailer, err := NewCodeMailer(scheduler)
			if err != nil {
				t.Fatal(err)
			}
			id := uuid.New()
			if err := mailer.SendCode(context.Background(), identity.CodeMessage{ID: id, Purpose: purpose, Destination: "buyer@example.com", Code: "012345", Lifetime: 90 * time.Second}); err != nil {
				t.Fatal(err)
			}
			if scheduler.jobType != template || scheduler.email != "buyer@example.com" || scheduler.locale != "" {
				t.Fatalf("scheduled (%q, %q, %q), want %q to the address in the store's locale", scheduler.jobType, scheduler.locale, scheduler.email, template)
			}
			encoded, err := json.Marshal(scheduler.payload)
			if err != nil {
				t.Fatal(err)
			}
			// A lifetime is rounded up, so the email never promises less time
			// than the code has.
			if want := `{"CodeID":"` + id.String() + `","Code":"012345","ExpiresInMinutes":2}`; string(encoded) != want {
				t.Fatalf("payload = %s, want %s", encoded, want)
			}
		})
	}
	if mailer, _ := NewCodeMailer(&schedulerFake{}); mailer.Channel() != identity.CodeChannelEmail {
		t.Fatalf("Channel() = %q", mailer.Channel())
	}
}

func TestTheMailerRefusesWhatItCannotSend(t *testing.T) {
	if _, err := NewCodeMailer(nil); err == nil {
		t.Fatal("NewCodeMailer(nil) succeeded")
	}
	scheduler := &schedulerFake{}
	mailer, _ := NewCodeMailer(scheduler)
	valid := identity.CodeMessage{ID: uuid.New(), Purpose: identity.CodePurposeSignIn, Destination: "buyer@example.com", Code: "123456", Lifetime: time.Minute}
	for name, mutate := range map[string]func(*identity.CodeMessage){
		"no id":       func(m *identity.CodeMessage) { m.ID = uuid.Nil },
		"no address":  func(m *identity.CodeMessage) { m.Destination = " " },
		"no code":     func(m *identity.CodeMessage) { m.Code = "" },
		"no lifetime": func(m *identity.CodeMessage) { m.Lifetime = 0 },
		"no purpose":  func(m *identity.CodeMessage) { m.Purpose = "" },
	} {
		message := valid
		mutate(&message)
		if err := mailer.SendCode(context.Background(), message); err == nil {
			t.Fatalf("%s: SendCode() succeeded", name)
		}
	}
	if scheduler.calls != 0 {
		t.Fatal("an invalid message was queued")
	}
}
