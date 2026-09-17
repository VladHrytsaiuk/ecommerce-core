package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

const codeSecret = "a-secure-secret-with-at-least-thirty-two-characters"

// codeStoreFake keeps the newest code per address, as the real store does, and
// counts what it was asked.
type codeStoreFake struct {
	issued   []domain.IssueCode
	codes    map[string][]byte
	consumed int
	issueErr error
}

func codeKey(purpose domain.CodePurpose, destinationHash []byte) string {
	return string(purpose) + ":" + hex.EncodeToString(destinationHash)
}

// inCodeTransaction marks the context the fake store hands to within, standing
// in for the transaction the real store carries.
type inCodeTransaction struct{}

func (f *codeStoreFake) Issue(ctx context.Context, request domain.IssueCode, within func(context.Context) error) error {
	if f.issueErr != nil && !request.Unthrottled {
		return f.issueErr
	}
	if err := within(context.WithValue(ctx, inCodeTransaction{}, true)); err != nil {
		return err
	}
	f.issued = append(f.issued, request)
	if f.codes == nil {
		f.codes = map[string][]byte{}
	}
	f.codes[codeKey(request.Purpose, request.DestinationHash)] = request.CodeHash
	return nil
}

func (f *codeStoreFake) Consume(ctx context.Context, request domain.ConsumeCode, onAccepted func(context.Context) error) (bool, error) {
	f.consumed++
	key := codeKey(request.Purpose, request.DestinationHash)
	stored, ok := f.codes[key]
	if !ok || !bytes.Equal(stored, request.CodeHash) {
		return false, nil
	}
	if err := onAccepted(ctx); err != nil {
		return false, err
	}
	delete(f.codes, key)
	return true, nil
}

func (f *codeStoreFake) PurgeSettled(context.Context, time.Time, int) (int, error) { return 0, nil }

type codeAccountsFake struct {
	byEmail   map[string]*domain.User
	claimed   []uuid.UUID
	passwords map[uuid.UUID]string
	// addressChanged makes VerifyEmail find the account's address different
	// from the one the code was sent to.
	addressChanged bool
}

func (f *codeAccountsFake) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	user, ok := f.byEmail[email]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	copied := *user
	return &copied, nil
}

func (f *codeAccountsFake) VerifyEmail(_ context.Context, userID uuid.UUID, email string) (bool, error) {
	if f.addressChanged {
		return false, nil
	}
	user, ok := f.byEmail[email]
	if !ok || user.ID != userID {
		return false, nil
	}
	user.EmailVerified = true
	return true, nil
}

func (f *codeAccountsFake) SetPassword(_ context.Context, userID uuid.UUID, passwordHash string) error {
	if f.passwords == nil {
		f.passwords = map[uuid.UUID]string{}
	}
	f.passwords[userID] = passwordHash
	for _, user := range f.byEmail {
		if user.ID == userID {
			user.PasswordHash, user.EmailVerified = passwordHash, true
			return nil
		}
	}
	return domain.ErrUserNotFound
}

func (f *codeAccountsFake) FindOrCreateByEmail(_ context.Context, email string) (*domain.User, error) {
	if user, ok := f.byEmail[email]; ok {
		copied := *user
		return &copied, nil
	}
	user := &domain.User{ID: uuid.New(), Email: &email, EmailVerified: true, Role: domain.RoleCustomer, Status: domain.UserStatusActive}
	f.byEmail[email] = user
	copied := *user
	return &copied, nil
}

func (f *codeAccountsFake) ClaimEmail(_ context.Context, userID uuid.UUID) error {
	f.claimed = append(f.claimed, userID)
	return nil
}

type codeSenderFake struct {
	channel domain.CodeChannel
	sent    []domain.CodeMessage
	err     error
}

func (f *codeSenderFake) Channel() domain.CodeChannel { return f.channel }

func (f *codeSenderFake) SendCode(_ context.Context, message domain.CodeMessage) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, message)
	return nil
}

type codeFixture struct {
	service  *AuthService
	users    *userRepositoryFake
	store    *codeStoreFake
	accounts *codeAccountsFake
	sender   *codeSenderFake
	refresh  *refreshStoreFake
}

func newCodeFixture(t *testing.T) codeFixture {
	t.Helper()
	fixture := codeFixture{
		store:    &codeStoreFake{},
		accounts: &codeAccountsFake{byEmail: map[string]*domain.User{}},
		sender:   &codeSenderFake{channel: domain.CodeChannelEmail},
		refresh:  &refreshStoreFake{},
	}
	fixture.users = &userRepositoryFake{byLogin: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}}
	fixture.service = refreshService(t, fixture.users, fixture.refresh)
	if _, err := fixture.service.WithCodes(fixture.store, fixture.accounts, codeSecret, 10*time.Minute, fixture.sender); err != nil {
		t.Fatal(err)
	}
	fixture.service.WithCodeSignIn(true)
	return fixture
}

func (f codeFixture) request(t *testing.T, email string) domain.CodeMessage {
	t.Helper()
	before := len(f.sender.sent)
	if _, err := f.service.RequestSignInCode(context.Background(), domain.RequestSignInCodeCommand{Channel: domain.CodeChannelEmail, Destination: email}); err != nil {
		t.Fatalf("RequestSignInCode(%q) error = %v", email, err)
	}
	if len(f.sender.sent) != before+1 {
		t.Fatalf("RequestSignInCode(%q) sent %d messages, want one", email, len(f.sender.sent)-before)
	}
	return f.sender.sent[len(f.sender.sent)-1]
}

func (f codeFixture) verify(email, code string) (domain.Session, error) {
	return f.service.VerifySignInCode(context.Background(), domain.VerifySignInCodeCommand{Channel: domain.CodeChannelEmail, Destination: email, Code: code})
}

func TestARequestedCodeIsSentAndOnlyItsHashesAreStored(t *testing.T) {
	fixture := newCodeFixture(t)

	issued, err := fixture.service.RequestSignInCode(context.Background(), domain.RequestSignInCodeCommand{Channel: domain.CodeChannelEmail, Destination: "  Buyer@Example.COM "})
	if err != nil {
		t.Fatalf("RequestSignInCode() error = %v", err)
	}
	if len(fixture.sender.sent) != 1 || len(fixture.store.issued) != 1 {
		t.Fatalf("sent %d, stored %d; want one of each", len(fixture.sender.sent), len(fixture.store.issued))
	}
	message, stored := fixture.sender.sent[0], fixture.store.issued[0]
	if message.Destination != "buyer@example.com" || !wellFormedCode(message.Code) || message.Lifetime != 10*time.Minute || message.ID != stored.ID {
		t.Fatalf("message = %+v, want the normalized address, a six-digit code, its lifetime and the stored id", message)
	}
	if len(stored.DestinationHash) != 32 || len(stored.CodeHash) != 32 ||
		bytes.Contains(stored.DestinationHash, []byte("buyer")) || bytes.Contains(stored.CodeHash, []byte(message.Code)) {
		t.Fatalf("stored = %+v, want only 32-byte HMACs", stored)
	}
	if !stored.ExpiresAt.Equal(refreshClock.Add(10*time.Minute)) || !issued.ExpiresAt.Equal(stored.ExpiresAt) || !issued.ResendAfter.Equal(refreshClock.Add(time.Minute)) {
		t.Fatalf("issued = %+v, stored expiry %s; want ten minutes to expire and one to resend", issued, stored.ExpiresAt)
	}
	if stored.Limits != codeLimits {
		t.Fatalf("limits = %+v, want %+v", stored.Limits, codeLimits)
	}
}

func TestTheEmailedCodeRegistersANewAddress(t *testing.T) {
	fixture := newCodeFixture(t)
	message := fixture.request(t, "Buyer@Example.com")

	// The address is typed differently the second time; it is the same address.
	session, err := fixture.verify("buyer@example.com ", message.Code)
	if err != nil {
		t.Fatalf("VerifySignInCode() error = %v", err)
	}
	created := fixture.accounts.byEmail["buyer@example.com"]
	if created == nil || session.UserID != created.ID || session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatalf("session = %+v, account = %+v; want a signed-in new account", session, created)
	}
	if len(fixture.accounts.claimed) != 0 || len(fixture.refresh.revokedUsers) != 0 {
		t.Fatal("a new account was claimed or had its sign-ins revoked")
	}
	if _, err := fixture.verify("buyer@example.com", message.Code); !errors.Is(err, domain.ErrInvalidCode) {
		t.Fatalf("second use of a code = %v, want ErrInvalidCode", err)
	}
}

func TestAWrongCodeIsRefused(t *testing.T) {
	fixture := newCodeFixture(t)
	message := fixture.request(t, "buyer@example.com")
	wrong := "000000"
	if message.Code == wrong {
		wrong = "000001"
	}

	if _, err := fixture.verify("buyer@example.com", wrong); !errors.Is(err, domain.ErrInvalidCode) {
		t.Fatalf("VerifySignInCode(wrong) = %v, want ErrInvalidCode", err)
	}
	if len(fixture.accounts.byEmail) != 0 {
		t.Fatal("an account was created without a valid code")
	}
}

func TestACodeIsBoundToTheAddressItWasSentTo(t *testing.T) {
	codes := newCodeFixture(t).service.codes
	buyer := codes.destinationHash(domain.CodeChannelEmail, "buyer@example.com")
	other := codes.destinationHash(domain.CodeChannelEmail, "other@example.com")
	if bytes.Equal(buyer, other) {
		t.Fatal("two addresses share a destination hash")
	}
	if bytes.Equal(codes.codeHash(buyer, domain.CodePurposeSignIn, "123456"), codes.codeHash(other, domain.CodePurposeSignIn, "123456")) {
		t.Fatal("the same digits hash alike for two addresses; a code would work for an address it was not sent to")
	}
	if bytes.Equal(codes.codeHash(buyer, domain.CodePurposeSignIn, "123456"), codes.codeHash(buyer, domain.CodePurposeResetPassword, "123456")) {
		t.Fatal("the same digits hash alike for two purposes; a sign-in code would reset a password")
	}
	// Keys come from the secret: another store's secret yields other hashes.
	service := refreshService(t, &userRepositoryFake{}, nil)
	if _, err := service.WithCodes(&codeStoreFake{}, &codeAccountsFake{}, codeSecret+"-other", 10*time.Minute, &codeSenderFake{channel: domain.CodeChannelEmail}); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(service.codes.destinationHash(domain.CodeChannelEmail, "buyer@example.com"), buyer) {
		t.Fatal("destination hashes do not depend on the secret")
	}
}

func TestAMalformedCodeOrAddressNeverReachesTheStore(t *testing.T) {
	fixture := newCodeFixture(t)
	for name, testCase := range map[string]struct{ email, code string }{
		"five digits":         {"buyer@example.com", "12345"},
		"seven digits":        {"buyer@example.com", "1234567"},
		"letters":             {"buyer@example.com", "12a456"},
		"full-width digits":   {"buyer@example.com", "１２３４５６"},
		"not an address":      {"buyer", "123456"},
		"a display name":      {"Buyer <buyer@example.com>", "123456"},
		"an overlong address": {strings.Repeat("a", 310) + "@example.com", "123456"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := fixture.verify(testCase.email, testCase.code); !errors.Is(err, domain.ErrInvalidCode) {
				t.Fatalf("VerifySignInCode(%q, %q) = %v, want ErrInvalidCode", testCase.email, testCase.code, err)
			}
		})
	}
	if fixture.store.consumed != 0 {
		t.Fatalf("the store was asked %d times; a malformed request must not use up an attempt", fixture.store.consumed)
	}
}

func TestACodeIsNotSentToSomethingThatIsNotAnAddress(t *testing.T) {
	fixture := newCodeFixture(t)
	for _, destination := range []string{"", "   ", "buyer", "buyer@", "Buyer <buyer@example.com>", "a@b@example.com"} {
		_, err := fixture.service.RequestSignInCode(context.Background(), domain.RequestSignInCodeCommand{Channel: domain.CodeChannelEmail, Destination: destination})
		if !errors.Is(err, domain.ErrInvalidCodeDestination) {
			t.Fatalf("RequestSignInCode(%q) = %v, want ErrInvalidCodeDestination", destination, err)
		}
	}
	if len(fixture.sender.sent) != 0 || len(fixture.store.issued) != 0 {
		t.Fatal("a code was issued for an invalid address")
	}
}

func TestAnUnverifiedAccountIsClaimedByWhoeverReceivesTheCode(t *testing.T) {
	// Someone registered the address with a password before its owner arrived.
	fixture := newCodeFixture(t)
	email := "buyer@example.com"
	squatted := &domain.User{ID: uuid.New(), Email: &email, PasswordHash: "squatter-hash", Role: domain.RoleCustomer, Status: domain.UserStatusActive}
	fixture.accounts.byEmail[email] = squatted
	message := fixture.request(t, email)

	session, err := fixture.verify(email, message.Code)
	if err != nil {
		t.Fatalf("VerifySignInCode() error = %v", err)
	}
	if session.UserID != squatted.ID {
		t.Fatalf("signed in to %s, want the existing account %s", session.UserID, squatted.ID)
	}
	if len(fixture.accounts.claimed) != 1 || fixture.accounts.claimed[0] != squatted.ID {
		t.Fatalf("claimed = %v, want the account's address verified and its password removed", fixture.accounts.claimed)
	}
	if len(fixture.refresh.revokedUsers) != 1 || fixture.refresh.revokedUsers[0] != squatted.ID {
		t.Fatalf("revoked = %v, want every earlier sign-in of the account ended", fixture.refresh.revokedUsers)
	}
}

func TestAVerifiedAccountIsSignedInWithoutBeingClaimed(t *testing.T) {
	fixture := newCodeFixture(t)
	email := "buyer@example.com"
	owner := &domain.User{ID: uuid.New(), Email: &email, EmailVerified: true, PasswordHash: "owner-hash", Role: domain.RoleManager, Status: domain.UserStatusActive}
	fixture.accounts.byEmail[email] = owner
	message := fixture.request(t, email)

	session, err := fixture.verify(email, message.Code)
	if err != nil || session.UserID != owner.ID || session.Role != domain.RoleManager {
		t.Fatalf("VerifySignInCode() = (%+v, %v), want the owner's session with their role", session, err)
	}
	if len(fixture.accounts.claimed) != 0 || len(fixture.refresh.revokedUsers) != 0 {
		t.Fatal("a verified account lost its password or its other sign-ins")
	}
}

func TestAnInactiveAccountCannotSignInWithACode(t *testing.T) {
	fixture := newCodeFixture(t)
	email := "buyer@example.com"
	disabled := &domain.User{ID: uuid.New(), Email: &email, EmailVerified: true, Role: domain.RoleCustomer, Status: domain.UserStatusDisabled}
	fixture.accounts.byEmail[email] = disabled
	message := fixture.request(t, email)

	session, err := fixture.verify(email, message.Code)
	if !errors.Is(err, domain.ErrInvalidCredentials) || session.AccessToken != "" {
		t.Fatalf("VerifySignInCode() = (%+v, %v), want refused", session, err)
	}
	if len(fixture.refresh.created) != 0 {
		t.Fatal("a disabled account was issued a refresh token")
	}
}

func TestAFailedDeliveryIssuesNoCode(t *testing.T) {
	fixture := newCodeFixture(t)
	fixture.sender.err = errors.New("queue unavailable")

	if _, err := fixture.service.RequestSignInCode(context.Background(), domain.RequestSignInCodeCommand{Channel: domain.CodeChannelEmail, Destination: "buyer@example.com"}); err == nil {
		t.Fatal("RequestSignInCode() succeeded although the code could not be queued")
	}
	if len(fixture.store.issued) != 0 {
		t.Fatal("a code was stored that nobody was sent")
	}
}

func TestAThrottledRequestIsReportedAsSuch(t *testing.T) {
	fixture := newCodeFixture(t)
	fixture.store.issueErr = &domain.CodeThrottledError{RetryAfter: 42 * time.Second}

	_, err := fixture.service.RequestSignInCode(context.Background(), domain.RequestSignInCodeCommand{Channel: domain.CodeChannelEmail, Destination: "buyer@example.com"})
	var throttled *domain.CodeThrottledError
	if !errors.As(err, &throttled) || throttled.RetryAfter != 42*time.Second {
		t.Fatalf("RequestSignInCode() = %v, want the throttle and its wait", err)
	}
}

func TestCodeSignInIsUnavailableUntilEnabled(t *testing.T) {
	service := refreshService(t, &userRepositoryFake{byLogin: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}}, nil)

	if _, err := service.RequestSignInCode(context.Background(), domain.RequestSignInCodeCommand{Channel: domain.CodeChannelEmail, Destination: "buyer@example.com"}); !errors.Is(err, domain.ErrSignInMethodDisabled) {
		t.Fatalf("RequestSignInCode() = %v, want ErrSignInMethodDisabled", err)
	}
	if _, err := service.VerifySignInCode(context.Background(), domain.VerifySignInCodeCommand{Channel: domain.CodeChannelEmail, Destination: "buyer@example.com", Code: "123456"}); !errors.Is(err, domain.ErrSignInMethodDisabled) {
		t.Fatalf("VerifySignInCode() = %v, want ErrSignInMethodDisabled", err)
	}
	fixture := newCodeFixture(t)
	if _, err := fixture.service.RequestSignInCode(context.Background(), domain.RequestSignInCodeCommand{Channel: "phone", Destination: "+380501234567"}); !errors.Is(err, domain.ErrSignInMethodDisabled) {
		t.Fatalf("RequestSignInCode(phone) = %v, want a channel with no sender refused", err)
	}
}

func TestSignInCodesRefuseAConfigurationThatCannotWork(t *testing.T) {
	store, accounts := &codeStoreFake{}, &codeAccountsFake{}
	email := &codeSenderFake{channel: domain.CodeChannelEmail}
	for name, testCase := range map[string]struct {
		secret  string
		ttl     time.Duration
		senders []domain.CodeSender
	}{
		"no sender":            {codeSecret, 10 * time.Minute, nil},
		"no secret":            {"  ", 10 * time.Minute, []domain.CodeSender{email}},
		"a lifetime too short": {codeSecret, 59 * time.Second, []domain.CodeSender{email}},
		"a lifetime too long":  {codeSecret, 31 * time.Minute, []domain.CodeSender{email}},
		"two email senders":    {codeSecret, 10 * time.Minute, []domain.CodeSender{email, &codeSenderFake{channel: domain.CodeChannelEmail}}},
		"an unknown channel":   {codeSecret, 10 * time.Minute, []domain.CodeSender{&codeSenderFake{channel: "pigeon"}}},
	} {
		t.Run(name, func(t *testing.T) {
			service := refreshService(t, &userRepositoryFake{}, nil)
			if _, err := service.WithCodes(store, accounts, testCase.secret, testCase.ttl, testCase.senders...); err == nil {
				t.Fatal("WithCodes() accepted a configuration that cannot work")
			}
			if service.codes != nil {
				t.Fatal("a refused configuration was still enabled")
			}
		})
	}
}

func TestCodesAreSixDigitsAndDoNotRepeatInSequence(t *testing.T) {
	seen := map[string]struct{}{}
	for range 200 {
		code, err := newCode()
		if err != nil {
			t.Fatal(err)
		}
		if !wellFormedCode(code) {
			t.Fatalf("newCode() = %q, want six ASCII digits", code)
		}
		seen[code] = struct{}{}
	}
	// 200 draws from a million collide rarely; a handful of distinct values
	// would mean the generator is not random at all.
	if len(seen) < 190 {
		t.Fatalf("200 codes had only %d distinct values", len(seen))
	}
}
