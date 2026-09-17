package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
)

// seedAccount puts one account where both the user repository and the code
// account resolver find it, as the database would.
func (f codeFixture) seedAccount(email string, verified bool, status domain.UserStatus) *domain.User {
	user := &domain.User{ID: uuid.New(), Email: &email, EmailVerified: verified, PasswordHash: "old-hash", Role: domain.RoleCustomer, Status: status}
	f.users.byLogin[email], f.users.byID[user.ID] = user, user
	f.accounts.byEmail[email] = user
	return user
}

func (f codeFixture) lastSent(t *testing.T, purpose domain.CodePurpose) domain.CodeMessage {
	t.Helper()
	if len(f.sender.sent) == 0 {
		t.Fatal("no code was sent")
	}
	message := f.sender.sent[len(f.sender.sent)-1]
	if message.Purpose != purpose {
		t.Fatalf("last code was for %q, want %q", message.Purpose, purpose)
	}
	return message
}

func TestRegisteringWithAnAddressSendsACodeToConfirmIt(t *testing.T) {
	fixture := newCodeFixture(t)
	email := " Buyer@Example.com "

	session, err := fixture.service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Email: &email, Password: "correct-horse-battery"})
	if err != nil || session.AccessToken == "" {
		t.Fatalf("RegisterPassword() = (%+v, %v), want a session", session, err)
	}
	message := fixture.lastSent(t, domain.CodePurposeVerifyEmail)
	if message.Destination != "buyer@example.com" {
		t.Fatalf("code sent to %q, want the normalized address", message.Destination)
	}
	stored := fixture.store.issued[len(fixture.store.issued)-1]
	if !stored.Unthrottled || stored.Purpose != domain.CodePurposeVerifyEmail {
		t.Fatalf("stored = %+v, want an unthrottled verification code", stored)
	}
	if _, ok := fixture.users.byLogin["buyer@example.com"]; !ok {
		t.Fatal("the account was not created")
	}
}

func TestRegistrationIsNotRefusedBecauseTheAddressWasSentCodesRecently(t *testing.T) {
	fixture := newCodeFixture(t)
	fixture.store.issueErr = &domain.CodeThrottledError{RetryAfter: time.Minute}
	email := "buyer@example.com"

	if _, err := fixture.service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Email: &email, Password: "correct-horse-battery"}); err != nil {
		t.Fatalf("RegisterPassword() = %v, want registration to go ahead", err)
	}
}

func TestAFailedRegistrationStoresNoCode(t *testing.T) {
	fixture := newCodeFixture(t)
	fixture.seedAccount("buyer@example.com", false, domain.UserStatusActive)
	email := "buyer@example.com"

	if _, err := fixture.service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Email: &email, Password: "correct-horse-battery"}); !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("RegisterPassword() = %v, want the address taken", err)
	}
	if len(fixture.store.issued) != 0 || len(fixture.sender.sent) != 0 {
		t.Fatal("a registration that failed left a code behind")
	}
}

func TestRegistrationSendsNoCodeWhereThereIsNothingToSendItWith(t *testing.T) {
	users := &userRepositoryFake{byLogin: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}}
	service := refreshService(t, users, nil)
	email, phone := "buyer@example.com", "+380501234567"

	if _, err := service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Email: &email, Password: "correct-horse-battery"}); err != nil {
		t.Fatalf("RegisterPassword() without codes = %v, want registration as before", err)
	}
	fixture := newCodeFixture(t)
	if _, err := fixture.service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Phone: &phone, Password: "correct-horse-battery"}); err != nil {
		t.Fatal(err)
	}
	if len(fixture.sender.sent) != 0 {
		t.Fatal("a phone-only registration was sent an email code")
	}
}

func TestRegistrationRefusesSomethingThatIsNotAnAddress(t *testing.T) {
	fixture := newCodeFixture(t)
	for _, email := range []string{"buyer", "buyer@", "Buyer <buyer@example.com>"} {
		if _, err := fixture.service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Email: &email, Password: "correct-horse-battery"}); !errors.Is(err, domain.ErrInvalidEmail) {
			t.Fatalf("RegisterPassword(%q) = %v, want ErrInvalidEmail", email, err)
		}
	}
	if len(fixture.users.byLogin) != 0 {
		t.Fatal("an account was created with an invalid address")
	}
}

func TestTheEmailedCodeConfirmsTheAddress(t *testing.T) {
	fixture := newCodeFixture(t)
	user := fixture.seedAccount("buyer@example.com", false, domain.UserStatusActive)

	if _, err := fixture.service.RequestEmailVerification(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
	message := fixture.lastSent(t, domain.CodePurposeVerifyEmail)
	if err := fixture.service.ConfirmEmail(context.Background(), domain.ConfirmEmailCommand{UserID: user.ID, Code: "999999x"}); !errors.Is(err, domain.ErrInvalidCode) {
		t.Fatalf("ConfirmEmail(malformed) = %v, want ErrInvalidCode", err)
	}
	if err := fixture.service.ConfirmEmail(context.Background(), domain.ConfirmEmailCommand{UserID: user.ID, Code: message.Code}); err != nil {
		t.Fatalf("ConfirmEmail() = %v", err)
	}
	if !user.EmailVerified {
		t.Fatal("the address was not marked verified")
	}
	if user.PasswordHash != "old-hash" || len(fixture.refresh.revokedUsers) != 0 {
		t.Fatal("confirming an address changed the password or ended sign-ins")
	}
}

func TestACodeForOnePurposeDoesNotServeAnother(t *testing.T) {
	fixture := newCodeFixture(t)
	user := fixture.seedAccount("buyer@example.com", false, domain.UserStatusActive)
	signIn := fixture.request(t, "buyer@example.com")

	if err := fixture.service.ConfirmEmail(context.Background(), domain.ConfirmEmailCommand{UserID: user.ID, Code: signIn.Code}); !errors.Is(err, domain.ErrInvalidCode) {
		t.Fatalf("ConfirmEmail(sign-in code) = %v, want ErrInvalidCode", err)
	}
	if _, err := fixture.service.ResetPassword(context.Background(), domain.ResetPasswordCommand{Email: "buyer@example.com", Code: signIn.Code, Password: "new-password-123"}); !errors.Is(err, domain.ErrInvalidCode) {
		t.Fatalf("ResetPassword(sign-in code) = %v, want ErrInvalidCode", err)
	}
}

func TestVerificationIsRefusedWhereItCannotApply(t *testing.T) {
	fixture := newCodeFixture(t)
	verified := fixture.seedAccount("verified@example.com", true, domain.UserStatusActive)
	disabled := fixture.seedAccount("disabled@example.com", false, domain.UserStatusDisabled)
	phoneOnly := &domain.User{ID: uuid.New(), Role: domain.RoleCustomer, Status: domain.UserStatusActive}
	fixture.users.byID[phoneOnly.ID] = phoneOnly

	for name, testCase := range map[string]struct {
		userID uuid.UUID
		want   error
	}{
		"already verified": {verified.ID, domain.ErrEmailAlreadyVerified},
		"disabled":         {disabled.ID, domain.ErrInvalidCredentials},
		"no address":       {phoneOnly.ID, domain.ErrNoEmailToVerify},
		"no account":       {uuid.New(), domain.ErrInvalidCredentials},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := fixture.service.RequestEmailVerification(context.Background(), testCase.userID); !errors.Is(err, testCase.want) {
				t.Fatalf("RequestEmailVerification() = %v, want %v", err, testCase.want)
			}
		})
	}
	if len(fixture.sender.sent) != 0 {
		t.Fatal("a verification code was sent where none applies")
	}
	withoutCodes := refreshService(t, fixture.users, nil)
	if _, err := withoutCodes.RequestEmailVerification(context.Background(), verified.ID); !errors.Is(err, domain.ErrCodesUnavailable) {
		t.Fatalf("RequestEmailVerification() without codes = %v, want ErrCodesUnavailable", err)
	}
}

func TestAPasswordResetAnswersAlikeForEveryAddress(t *testing.T) {
	fixture := newCodeFixture(t)
	fixture.seedAccount("buyer@example.com", false, domain.UserStatusActive)
	fixture.seedAccount("disabled@example.com", true, domain.UserStatusDisabled)

	for _, email := range []string{"buyer@example.com", "nobody@example.com", "disabled@example.com"} {
		issued, err := fixture.service.RequestPasswordReset(context.Background(), domain.RequestPasswordResetCommand{Email: email})
		if err != nil || !issued.ExpiresAt.Equal(refreshClock.Add(10*time.Minute)) {
			t.Fatalf("RequestPasswordReset(%q) = (%+v, %v), want the same accepted answer", email, issued, err)
		}
	}
	// A code is stored for all three, so the limits treat them alike; only the
	// account that can sign in is sent one.
	if len(fixture.store.issued) != 3 || len(fixture.sender.sent) != 1 || fixture.sender.sent[0].Destination != "buyer@example.com" {
		t.Fatalf("stored %d, sent %+v; want three stored and one sent, to the account", len(fixture.store.issued), fixture.sender.sent)
	}
}

func TestAResetCodeSetsANewPasswordAndEndsEveryOtherSignIn(t *testing.T) {
	fixture := newCodeFixture(t)
	user := fixture.seedAccount("buyer@example.com", false, domain.UserStatusActive)
	if _, err := fixture.service.RequestPasswordReset(context.Background(), domain.RequestPasswordResetCommand{Email: "Buyer@Example.com"}); err != nil {
		t.Fatal(err)
	}
	message := fixture.lastSent(t, domain.CodePurposeResetPassword)

	session, err := fixture.service.ResetPassword(context.Background(), domain.ResetPasswordCommand{Email: "buyer@example.com", Code: message.Code, Password: "new-password-123"})
	if err != nil || session.UserID != user.ID || session.RefreshToken == "" {
		t.Fatalf("ResetPassword() = (%+v, %v), want a session for the account", session, err)
	}
	if password.CheckPassword("new-password-123", fixture.accounts.passwords[user.ID]) != nil {
		t.Fatal("the new password was not stored")
	}
	if !user.EmailVerified {
		t.Fatal("a reset read at the address did not verify it")
	}
	if len(fixture.refresh.revokedUsers) != 1 || fixture.refresh.revokedUsers[0] != user.ID {
		t.Fatalf("revoked = %v, want every earlier sign-in ended", fixture.refresh.revokedUsers)
	}
}

func TestAResetWithAnUnacceptablePasswordUsesUpNoAttempt(t *testing.T) {
	fixture := newCodeFixture(t)
	fixture.seedAccount("buyer@example.com", true, domain.UserStatusActive)

	if _, err := fixture.service.ResetPassword(context.Background(), domain.ResetPasswordCommand{Email: "buyer@example.com", Code: "123456", Password: "short"}); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatalf("ResetPassword(short password) = %v, want ErrInvalidPassword", err)
	}
	if fixture.store.consumed != 0 {
		t.Fatal("a refused password used up an attempt at the code")
	}
}

func TestAResetCannotBeUsedForAnAddressWithoutAnAccount(t *testing.T) {
	fixture := newCodeFixture(t)
	if _, err := fixture.service.RequestPasswordReset(context.Background(), domain.RequestPasswordResetCommand{Email: "nobody@example.com"}); err != nil {
		t.Fatal(err)
	}
	// Nothing was sent, so read the code the way only a test can: re-issue it
	// under a known value.
	stored := fixture.store.issued[0]
	codes := fixture.service.codes
	fixture.store.codes[codeKey(stored.Purpose, stored.DestinationHash)] = codes.codeHash(stored.DestinationHash, domain.CodePurposeResetPassword, "123456")

	if _, err := fixture.service.ResetPassword(context.Background(), domain.ResetPasswordCommand{Email: "nobody@example.com", Code: "123456", Password: "new-password-123"}); !errors.Is(err, domain.ErrInvalidCode) {
		t.Fatalf("ResetPassword(no account) = %v, want ErrInvalidCode", err)
	}
	if len(fixture.accounts.passwords) != 0 {
		t.Fatal("a password was set for an address with no account")
	}
}

func TestPasswordResetFollowsThePasswordMethod(t *testing.T) {
	fixture := newCodeFixture(t)
	fixture.service.WithPasswordSignIn(false)

	if _, err := fixture.service.RequestPasswordReset(context.Background(), domain.RequestPasswordResetCommand{Email: "buyer@example.com"}); !errors.Is(err, domain.ErrSignInMethodDisabled) {
		t.Fatalf("RequestPasswordReset() with passwords off = %v, want ErrSignInMethodDisabled", err)
	}
	if _, err := fixture.service.ResetPassword(context.Background(), domain.ResetPasswordCommand{Email: "buyer@example.com", Code: "123456", Password: "new-password-123"}); !errors.Is(err, domain.ErrSignInMethodDisabled) {
		t.Fatalf("ResetPassword() with passwords off = %v, want ErrSignInMethodDisabled", err)
	}
}

func TestCodeSignInNeedsItsOwnSwitchAsWellAsCodes(t *testing.T) {
	fixture := newCodeFixture(t)
	fixture.service.WithCodeSignIn(domain.CodeChannelEmail, false)

	if _, err := fixture.service.RequestSignInCode(context.Background(), domain.RequestSignInCodeCommand{Channel: domain.CodeChannelEmail, Destination: "buyer@example.com"}); !errors.Is(err, domain.ErrSignInMethodDisabled) {
		t.Fatalf("RequestSignInCode() with code sign-in off = %v, want ErrSignInMethodDisabled", err)
	}
}

// transactionRecordingUsers notes whether an account was created in the
// transaction that stores its verification code.
type transactionRecordingUsers struct {
	*userRepositoryFake
	createdInCodeTransaction []bool
}

func (u *transactionRecordingUsers) Create(ctx context.Context, user domain.NewUser) (*domain.User, error) {
	inside, _ := ctx.Value(inCodeTransaction{}).(bool)
	u.createdInCodeTransaction = append(u.createdInCodeTransaction, inside)
	return u.userRepositoryFake.Create(ctx, user)
}

func TestRegistrationCreatesTheAccountInTheTransactionThatStoresItsCode(t *testing.T) {
	fixture := newCodeFixture(t)
	users := &transactionRecordingUsers{userRepositoryFake: fixture.users}
	fixture.service.users = users
	email := "buyer@example.com"

	if _, err := fixture.service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Email: &email, Password: "correct-horse-battery"}); err != nil {
		t.Fatal(err)
	}
	if len(users.createdInCodeTransaction) != 1 || !users.createdInCodeTransaction[0] {
		t.Fatalf("created in the code's transaction = %v; outside it, an account could exist whose code was never stored", users.createdInCodeTransaction)
	}
}

func TestAConfirmationIsRefusedIfTheAddressChangedAfterTheCodeWasSent(t *testing.T) {
	fixture := newCodeFixture(t)
	user := fixture.seedAccount("buyer@example.com", false, domain.UserStatusActive)
	if _, err := fixture.service.RequestEmailVerification(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
	message := fixture.lastSent(t, domain.CodePurposeVerifyEmail)
	fixture.accounts.addressChanged = true

	if err := fixture.service.ConfirmEmail(context.Background(), domain.ConfirmEmailCommand{UserID: user.ID, Code: message.Code}); !errors.Is(err, domain.ErrInvalidCode) {
		t.Fatalf("ConfirmEmail() = %v, want the code refused for an address the account no longer has", err)
	}
}
