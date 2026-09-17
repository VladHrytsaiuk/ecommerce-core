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

func TestTheMailerQueuesTheCodeWithTheTemplateItsDefaultReads(t *testing.T) {
	scheduler := &schedulerFake{}
	mailer, err := NewSignInCodeMailer(scheduler)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := mailer.SendSignInCode(context.Background(), identity.SignInCodeMessage{ID: id, Destination: "buyer@example.com", Code: "012345", Lifetime: 90 * time.Second}); err != nil {
		t.Fatal(err)
	}
	if scheduler.jobType != notifications.SignInCodeTemplate || scheduler.email != "buyer@example.com" || scheduler.locale != "" {
		t.Fatalf("scheduled (%q, %q, %q), want the sign-in template to the address in the store's locale", scheduler.jobType, scheduler.locale, scheduler.email)
	}
	encoded, err := json.Marshal(scheduler.payload)
	if err != nil {
		t.Fatal(err)
	}
	// A lifetime is rounded up, so the email never promises less time than the
	// code has.
	if want := `{"CodeID":"` + id.String() + `","Code":"012345","ExpiresInMinutes":2}`; string(encoded) != want {
		t.Fatalf("payload = %s, want %s", encoded, want)
	}
	if mailer.Channel() != identity.SignInCodeEmail {
		t.Fatalf("Channel() = %q", mailer.Channel())
	}
}

func TestTheMailerRefusesWhatItCannotSend(t *testing.T) {
	if _, err := NewSignInCodeMailer(nil); err == nil {
		t.Fatal("NewSignInCodeMailer(nil) succeeded")
	}
	scheduler := &schedulerFake{}
	mailer, _ := NewSignInCodeMailer(scheduler)
	valid := identity.SignInCodeMessage{ID: uuid.New(), Destination: "buyer@example.com", Code: "123456", Lifetime: time.Minute}
	for name, mutate := range map[string]func(*identity.SignInCodeMessage){
		"no id":       func(m *identity.SignInCodeMessage) { m.ID = uuid.Nil },
		"no address":  func(m *identity.SignInCodeMessage) { m.Destination = " " },
		"no code":     func(m *identity.SignInCodeMessage) { m.Code = "" },
		"no lifetime": func(m *identity.SignInCodeMessage) { m.Lifetime = 0 },
	} {
		message := valid
		mutate(&message)
		if err := mailer.SendSignInCode(context.Background(), message); err == nil {
			t.Fatalf("%s: SendSignInCode() succeeded", name)
		}
	}
	if scheduler.calls != 0 {
		t.Fatal("an invalid message was queued")
	}
}
