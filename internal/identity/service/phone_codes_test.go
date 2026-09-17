package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

type phoneFixture struct {
	codeFixture
	texter *codeSenderFake
}

func newPhoneFixture(t *testing.T) phoneFixture {
	t.Helper()
	fixture := phoneFixture{codeFixture: newCodeFixture(t), texter: &codeSenderFake{channel: domain.CodeChannelPhone}}
	if _, err := fixture.service.WithCodes(CodeConfig{
		Store: fixture.store, Accounts: fixture.accounts, Secret: codeSecret, TTL: 10 * time.Minute,
		Senders:           []domain.CodeSender{fixture.sender, fixture.texter},
		PhoneCountryCodes: []string{"380", "+48"}, PhoneHourlyLimit: 50,
	}); err != nil {
		t.Fatal(err)
	}
	fixture.service.WithCodeSignIn(domain.CodeChannelPhone, true)
	return fixture
}

func (f phoneFixture) requestPhone(phone string) (domain.CodeRequest, error) {
	return f.service.RequestSignInCode(context.Background(), domain.RequestSignInCodeCommand{Channel: domain.CodeChannelPhone, Destination: phone})
}

func (f phoneFixture) verifyPhone(phone, code string) (domain.Session, error) {
	return f.service.VerifySignInCode(context.Background(), domain.VerifySignInCodeCommand{Channel: domain.CodeChannelPhone, Destination: phone, Code: code})
}

func TestPhoneNumbersAreNormalizedToE164(t *testing.T) {
	for raw, want := range map[string]string{
		"+380501234567":        "+380501234567",
		" +380 (50) 123-45-67": "+380501234567",
		"00380501234567":       "+380501234567",
		"+48.600.700.800":      "+48600700800",
	} {
		if got, ok := normalizePhone(raw); !ok || got != want {
			t.Fatalf("normalizePhone(%q) = (%q, %v), want %q", raw, got, ok, want)
		}
	}
	for _, raw := range []string{"", "0501234567", "380501234567", "+0501234567", "+3805012", "+3805012345678901", "+38050123456a", "+380 50 123 45 67 ext 2"} {
		if got, ok := normalizePhone(raw); ok {
			t.Fatalf("normalizePhone(%q) = %q, want refused", raw, got)
		}
	}
}

func TestAPhoneCodeIsTextedAfterItIsStored(t *testing.T) {
	fixture := newPhoneFixture(t)

	issued, err := fixture.requestPhone("+380 50 123 45 67")
	if err != nil || !issued.ExpiresAt.Equal(refreshClock.Add(10*time.Minute)) {
		t.Fatalf("RequestSignInCode(phone) = (%+v, %v)", issued, err)
	}
	if len(fixture.texter.sent) != 1 || fixture.texter.sent[0].Destination != "+380501234567" {
		t.Fatalf("texts = %+v, want one to the normalized number", fixture.texter.sent)
	}
	// A provider's network call must not hold the transaction and the
	// number's advisory lock open.
	if fixture.texter.sentInTransaction[0] {
		t.Fatal("the text was sent inside the transaction that stores the code")
	}
	stored := fixture.store.issued[0]
	if stored.Channel != domain.CodeChannelPhone || stored.Limits.ChannelPerHour != 50 {
		t.Fatalf("stored = %+v, want a phone code under the store-wide hourly cap", stored)
	}
	if len(fixture.sender.sent) != 0 {
		t.Fatal("an email was sent for a phone code")
	}
}

func TestEmailCodesStayUnderNoStoreWideCap(t *testing.T) {
	fixture := newPhoneFixture(t)
	fixture.request(t, "buyer@example.com")
	if limits := fixture.store.issued[0].Limits; limits.ChannelPerHour != 0 {
		t.Fatalf("email limits = %+v, want no store-wide cap", limits)
	}
	if fixture.sender.sentInTransaction[0] != true {
		t.Fatal("an email code was not queued inside the transaction that stores it")
	}
}

func TestAPhoneCodeGoesOnlyToCountriesTheStoreTexts(t *testing.T) {
	fixture := newPhoneFixture(t)
	if _, err := fixture.requestPhone("+48600700800"); err != nil {
		t.Fatalf("RequestSignInCode(+48) = %v, want accepted", err)
	}
	for _, phone := range []string{"+12025550123", "+3805", "0501234567"} {
		if _, err := fixture.requestPhone(phone); !errors.Is(err, domain.ErrInvalidCodeDestination) {
			t.Fatalf("RequestSignInCode(%q) = %v, want ErrInvalidCodeDestination", phone, err)
		}
	}
	if _, err := fixture.verifyPhone("+12025550123", "123456"); !errors.Is(err, domain.ErrInvalidCode) {
		t.Fatalf("VerifySignInCode(outside the countries) = %v, want ErrInvalidCode", err)
	}
	if len(fixture.store.issued) != 1 || fixture.store.consumed != 0 {
		t.Fatalf("issued %d, consumed %d; want only the allowed number to reach the store", len(fixture.store.issued), fixture.store.consumed)
	}
}

func TestAFailedTextIsReportedAndTheCodeStillCounts(t *testing.T) {
	fixture := newPhoneFixture(t)
	fixture.texter.err = errors.New("provider down")

	if _, err := fixture.requestPhone("+380501234567"); !errors.Is(err, domain.ErrCodeDeliveryFailed) {
		t.Fatalf("RequestSignInCode() = %v, want ErrCodeDeliveryFailed", err)
	}
	if len(fixture.store.issued) != 1 {
		t.Fatal("the code was not kept; a failing provider would be called again at once")
	}
}

func TestATextedCodeRegistersANewNumber(t *testing.T) {
	fixture := newPhoneFixture(t)
	if _, err := fixture.requestPhone("+380501234567"); err != nil {
		t.Fatal(err)
	}

	session, err := fixture.verifyPhone("+380 50 123 45 67", fixture.texter.sent[0].Code)
	if err != nil {
		t.Fatalf("VerifySignInCode(phone) = %v", err)
	}
	created := fixture.accounts.byPhone["+380501234567"]
	if created == nil || session.UserID != created.ID || !created.PhoneVerified {
		t.Fatalf("session = %+v, account = %+v; want the new account with its number verified", session, created)
	}
	if _, err := fixture.verifyPhone("+380501234567", fixture.texter.sent[0].Code); !errors.Is(err, domain.ErrInvalidCode) {
		t.Fatalf("second use = %v, want ErrInvalidCode", err)
	}
}

func TestAnUnverifiedNumberIsClaimedByWhoeverReceivesTheCode(t *testing.T) {
	fixture := newPhoneFixture(t)
	phone := "+380501234567"
	squatted := &domain.User{ID: uuid.New(), Phone: &phone, PasswordHash: "squatter-hash", Role: domain.RoleCustomer, Status: domain.UserStatusActive}
	fixture.accounts.byPhone = map[string]*domain.User{phone: squatted}
	if _, err := fixture.requestPhone(phone); err != nil {
		t.Fatal(err)
	}

	session, err := fixture.verifyPhone(phone, fixture.texter.sent[0].Code)
	if err != nil || session.UserID != squatted.ID {
		t.Fatalf("VerifySignInCode() = (%+v, %v), want the existing account", session, err)
	}
	if len(fixture.accounts.claimed) != 1 || len(fixture.refresh.revokedUsers) != 1 {
		t.Fatalf("claimed %v, revoked %v; want the number claimed and earlier sign-ins ended", fixture.accounts.claimed, fixture.refresh.revokedUsers)
	}
}

func TestEachChannelsCodeSignInIsSwitchedSeparately(t *testing.T) {
	fixture := newPhoneFixture(t)
	fixture.service.WithCodeSignIn(domain.CodeChannelPhone, false)

	if _, err := fixture.requestPhone("+380501234567"); !errors.Is(err, domain.ErrSignInMethodDisabled) {
		t.Fatalf("RequestSignInCode(phone) with phone sign-in off = %v, want ErrSignInMethodDisabled", err)
	}
	fixture.request(t, "buyer@example.com")
}

func TestPhoneCodesRefuseAConfigurationThatInvitesFraud(t *testing.T) {
	texter := &codeSenderFake{channel: domain.CodeChannelPhone}
	for name, testCase := range map[string]struct {
		countryCodes []string
		hourlyLimit  int
	}{
		"no countries":      {nil, 50},
		"a malformed code":  {[]string{"38O"}, 50},
		"a four-digit code": {[]string{"3801"}, 50},
		"a leading zero":    {[]string{"038"}, 50},
		"no store-wide cap": {[]string{"380"}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			service := refreshService(t, &userRepositoryFake{}, nil)
			_, err := service.WithCodes(CodeConfig{Store: &codeStoreFake{}, Accounts: &codeAccountsFake{}, Secret: codeSecret, TTL: 10 * time.Minute, Senders: []domain.CodeSender{texter}, PhoneCountryCodes: testCase.countryCodes, PhoneHourlyLimit: testCase.hourlyLimit})
			if err == nil {
				t.Fatal("WithCodes() accepted phone codes that could be sent anywhere, without bound")
			}
		})
	}
}

func TestRegistrationAndPasswordSignInAgreeOnAPhoneNumber(t *testing.T) {
	users := &userRepositoryFake{byLogin: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}}
	service := refreshService(t, users, nil)
	formatted, invalid := "+380 (50) 123-45-67", "050 123 45 67"

	if _, err := service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Phone: &invalid, Password: "correct-horse-battery"}); !errors.Is(err, domain.ErrInvalidPhone) {
		t.Fatalf("RegisterPassword(no country code) = %v, want ErrInvalidPhone", err)
	}
	if _, err := service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Phone: &formatted, Password: "correct-horse-battery"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := users.byLogin["+380501234567"]; !ok {
		t.Fatal("the number was not stored in E.164 form")
	}
	if _, err := service.LoginPassword(context.Background(), domain.PasswordLoginCommand{Login: "00380 50 123 45 67", Password: "correct-horse-battery"}); err != nil {
		t.Fatalf("LoginPassword(the number written another way) = %v", err)
	}
}

// Which verification a code checks is its own channel's: a verified email does
// not vouch for an unproved phone number, nor the other way round.
func TestAClaimLooksAtTheChannelTheCodeWasSentOn(t *testing.T) {
	fixture := newPhoneFixture(t)
	phone, email := "+380501234567", "buyer@example.com"
	emailOnly := &domain.User{ID: uuid.New(), Phone: &phone, Email: &email, EmailVerified: true, PasswordHash: "hash", Role: domain.RoleCustomer, Status: domain.UserStatusActive}
	fixture.accounts.byPhone = map[string]*domain.User{phone: emailOnly}
	if _, err := fixture.requestPhone(phone); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.verifyPhone(phone, fixture.texter.sent[0].Code); err != nil {
		t.Fatal(err)
	}
	if len(fixture.accounts.claimed) != 1 {
		t.Fatal("an unverified number was not claimed because the account's email was verified")
	}

	fixture = newPhoneFixture(t)
	phoneOnly := &domain.User{ID: uuid.New(), Phone: &phone, PhoneVerified: true, PasswordHash: "hash", Role: domain.RoleCustomer, Status: domain.UserStatusActive}
	fixture.accounts.byPhone = map[string]*domain.User{phone: phoneOnly}
	if _, err := fixture.requestPhone(phone); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.verifyPhone(phone, fixture.texter.sent[0].Code); err != nil {
		t.Fatal(err)
	}
	if len(fixture.accounts.claimed) != 0 || len(fixture.refresh.revokedUsers) != 0 {
		t.Fatal("a verified number was claimed because the account has no verified email")
	}
}
