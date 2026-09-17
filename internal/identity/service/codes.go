package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"math/big"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/hkdf"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

const (
	codeDigits = 6
	// maxEmailLength is users.email's column width.
	maxEmailLength = 320

	codeDestinationKeyPurpose = "identity:one-time-code:destination:v1"
	codeKeyPurpose            = "identity:one-time-code:code:v1"
)

// codeLimits allow a person a resend a minute and a handful an hour, counted
// across every purpose. Five attempts per code, at most ten codes a day, leaves
// a guesser fifty tries a day at a million combinations — and fifty messages in
// the inbox of the person being targeted.
var codeLimits = domain.CodeLimits{
	Cooldown:    time.Minute,
	PerHour:     5,
	PerDay:      10,
	MaxAttempts: 5,
}

// MinCodeTTL and MaxCodeTTL bound how long a code works: long enough to switch
// to the mailbox and back, short enough that a code read over someone's
// shoulder is soon useless.
const (
	MinCodeTTL = time.Minute
	MaxCodeTTL = 30 * time.Minute
)

// oneTimeCodes issues and checks the codes behind sign-in by code, email
// verification and password reset.
type oneTimeCodes struct {
	store          domain.CodeStore
	accounts       domain.CodeAccounts
	senders        map[domain.CodeChannel]domain.CodeSender
	destinationKey []byte
	codeKey        []byte
	ttl            time.Duration
	// phoneCountryCodes are the calling codes a phone code may be sent to.
	phoneCountryCodes []string
	phoneHourlyLimit  int
}

// CodeConfig makes one-time codes available.
type CodeConfig struct {
	Store    domain.CodeStore
	Accounts domain.CodeAccounts
	// Secret is the server secret the hashing keys are derived from; it is
	// never used directly.
	Secret string
	TTL    time.Duration
	// Senders deliver codes, at most one per channel. The channels they cover
	// are the ones codes exist on.
	Senders []domain.CodeSender
	// PhoneCountryCodes are the international calling codes, without "+", a
	// phone code may be sent to. Required with a phone sender: a store that
	// texts any number in the world pays for whatever fraud finds it.
	PhoneCountryCodes []string
	// PhoneHourlyLimit caps the texts the whole store sends in an hour.
	// Required with a phone sender.
	PhoneHourlyLimit int
}

// WithCodes makes one-time codes available on the channels of the senders
// given. On email that enables verification and password reset; sign-in by
// code is enabled per channel, by WithCodeSignIn.
func (s *AuthService) WithCodes(config CodeConfig) (*AuthService, error) {
	if config.Store == nil || config.Accounts == nil || len(config.Senders) == 0 {
		return nil, fmt.Errorf("one-time codes require a store, an account resolver and at least one sender")
	}
	if strings.TrimSpace(config.Secret) == "" {
		return nil, fmt.Errorf("one-time codes require a server secret")
	}
	if config.TTL < MinCodeTTL || config.TTL > MaxCodeTTL {
		return nil, fmt.Errorf("one-time code lifetime must be between %s and %s", MinCodeTTL, MaxCodeTTL)
	}
	codes := &oneTimeCodes{store: config.Store, accounts: config.Accounts, ttl: config.TTL, senders: make(map[domain.CodeChannel]domain.CodeSender, len(config.Senders))}
	for _, sender := range config.Senders {
		if sender == nil {
			return nil, fmt.Errorf("one-time code sender is nil")
		}
		if sender.Channel() != domain.CodeChannelEmail && sender.Channel() != domain.CodeChannelPhone {
			return nil, fmt.Errorf("one-time code channel %q is not supported", sender.Channel())
		}
		if _, duplicate := codes.senders[sender.Channel()]; duplicate {
			return nil, fmt.Errorf("one-time code channel %q has more than one sender", sender.Channel())
		}
		codes.senders[sender.Channel()] = sender
	}
	if _, phone := codes.senders[domain.CodeChannelPhone]; phone {
		countryCodes, err := validCountryCodes(config.PhoneCountryCodes)
		if err != nil {
			return nil, err
		}
		if config.PhoneHourlyLimit < 1 {
			return nil, fmt.Errorf("phone codes require a store-wide hourly limit")
		}
		codes.phoneCountryCodes, codes.phoneHourlyLimit = countryCodes, config.PhoneHourlyLimit
	}
	var err error
	if codes.destinationKey, err = deriveKey(config.Secret, codeDestinationKeyPurpose); err != nil {
		return nil, err
	}
	if codes.codeKey, err = deriveKey(config.Secret, codeKeyPurpose); err != nil {
		return nil, err
	}
	s.codes = codes
	return s, nil
}

// WithCodeSignIn enables or disables signing in with a code on one channel. It
// has no effect on a channel WithCodes did not make available.
func (s *AuthService) WithCodeSignIn(channel domain.CodeChannel, enabled bool) *AuthService {
	if s.codeSignIn == nil {
		s.codeSignIn = make(map[domain.CodeChannel]bool)
	}
	s.codeSignIn[channel] = enabled
	return s
}

// RequestSignInCode sends a code to an address, whether or not it has an
// account: asking for a code is how an account is registered, and answering
// differently for a known address would say which addresses are known.
func (s *AuthService) RequestSignInCode(ctx context.Context, command domain.RequestSignInCodeCommand) (domain.CodeRequest, error) {
	if !s.codeSignInAvailable(command.Channel) {
		return domain.CodeRequest{}, domain.ErrSignInMethodDisabled
	}
	destination, ok := s.codes.normalizeDestination(command.Channel, command.Destination)
	if !ok {
		return domain.CodeRequest{}, domain.ErrInvalidCodeDestination
	}
	return s.codes.issue(ctx, s.now().UTC(), command.Channel, domain.CodePurposeSignIn, destination, false, nil)
}

// VerifySignInCode exchanges a code for a session. An address with no account
// gets one, already verified: receiving the code is the verification.
//
// An existing account whose address was never verified is claimed: the address
// is marked verified, the password is removed and every sign-in of the account
// is ended. Without that, anyone could register someone else's address with a
// password before its owner arrives, and keep a way in after the owner signs in
// with a code. The owner loses nothing — they sign in with codes, and may set a
// password again where the store offers one.
func (s *AuthService) VerifySignInCode(ctx context.Context, command domain.VerifySignInCodeCommand) (domain.Session, error) {
	if !s.codeSignInAvailable(command.Channel) {
		return domain.Session{}, domain.ErrSignInMethodDisabled
	}
	destination, ok := s.codes.normalizeDestination(command.Channel, command.Destination)
	if !ok {
		return domain.Session{}, domain.ErrInvalidCode
	}
	now := s.now().UTC()
	var user *domain.User
	err := s.codes.consume(ctx, now, command.Channel, domain.CodePurposeSignIn, destination, command.Code, func(txCtx context.Context) error {
		found, err := s.codes.findOrCreate(txCtx, command.Channel, destination)
		if err != nil {
			return err
		}
		if !canSignIn(found) {
			// Refused inside the transaction, so the code is not spent on an
			// account that cannot sign in.
			return domain.ErrInvalidCredentials
		}
		if !verifiedOn(found, command.Channel) {
			if err := s.codes.claim(txCtx, command.Channel, found.ID); err != nil {
				return err
			}
			if err := s.endEverySignIn(txCtx, found.ID, now); err != nil {
				return err
			}
			found.PasswordHash = ""
		}
		user = found
		return nil
	})
	if err != nil {
		return domain.Session{}, err
	}
	return s.finishLogin(ctx, user, command.GuestSessionID)
}

func (s *AuthService) codeSignInAvailable(channel domain.CodeChannel) bool {
	return s.codeSignIn[channel] && s.codesAvailable(channel)
}

func (s *AuthService) codesAvailable(channel domain.CodeChannel) bool {
	if s.codes == nil || s.tokens == nil || s.accessTTL <= 0 {
		return false
	}
	_, ok := s.codes.senders[channel]
	return ok
}

// endEverySignIn revokes the account's refresh tokens, joining the transaction
// in ctx. Access tokens already issued run out on their own.
func (s *AuthService) endEverySignIn(ctx context.Context, userID uuid.UUID, now time.Time) error {
	if s.refreshTokens == nil {
		return nil
	}
	return s.refreshTokens.RevokeUser(ctx, userID, now)
}

// issue stores a code for destination and sends it.
//
// within runs first, in the transaction that stores the code, and says whether
// to send it. A code is still stored when within declines, so a request for an
// address that will not be sent anything is counted and throttled exactly like
// one that will: otherwise the limits would tell which addresses have accounts.
//
// A transactional sender queues the message in that transaction. Any other is
// called once it commits, so no transaction or advisory lock is held across a
// provider's network call; if that call fails the code stays stored and
// counted, and the caller learns ErrCodeDeliveryFailed.
func (c *oneTimeCodes) issue(ctx context.Context, now time.Time, channel domain.CodeChannel, purpose domain.CodePurpose, destination string, unthrottled bool, within func(context.Context) (bool, error)) (domain.CodeRequest, error) {
	sender, ok := c.senders[channel]
	if !ok {
		return domain.CodeRequest{}, domain.ErrCodesUnavailable
	}
	code, err := newCode()
	if err != nil {
		return domain.CodeRequest{}, err
	}
	destinationHash := c.destinationHash(channel, destination)
	expiresAt := now.Add(c.ttl)
	message := domain.CodeMessage{ID: uuid.New(), Purpose: purpose, Destination: destination, Code: code, Lifetime: c.ttl}
	limits := codeLimits
	if channel == domain.CodeChannelPhone {
		limits.ChannelPerHour = c.phoneHourlyLimit
	}
	send := true
	err = c.store.Issue(ctx, domain.IssueCode{
		ID:              message.ID,
		Channel:         channel,
		Purpose:         purpose,
		DestinationHash: destinationHash,
		CodeHash:        c.codeHash(destinationHash, purpose, code),
		Now:             now,
		ExpiresAt:       expiresAt,
		Limits:          limits,
		Unthrottled:     unthrottled,
	}, func(txCtx context.Context) error {
		if within != nil {
			var err error
			if send, err = within(txCtx); err != nil {
				return err
			}
		}
		if !send || !sender.Transactional() {
			return nil
		}
		return sender.SendCode(txCtx, message)
	})
	if err != nil {
		return domain.CodeRequest{}, err
	}
	if send && !sender.Transactional() {
		if err := sender.SendCode(ctx, message); err != nil {
			return domain.CodeRequest{}, fmt.Errorf("%w: %w", domain.ErrCodeDeliveryFailed, err)
		}
	}
	return domain.CodeRequest{ExpiresAt: expiresAt, ResendAfter: now.Add(codeLimits.Cooldown)}, nil
}

// consume checks a code and runs onAccepted in the transaction that accepts it.
// A code that is malformed never reaches the store, so it uses up no attempt.
func (c *oneTimeCodes) consume(ctx context.Context, now time.Time, channel domain.CodeChannel, purpose domain.CodePurpose, destination, code string, onAccepted func(context.Context) error) error {
	code = strings.TrimSpace(code)
	if !wellFormedCode(code) {
		return domain.ErrInvalidCode
	}
	destinationHash := c.destinationHash(channel, destination)
	accepted, err := c.store.Consume(ctx, domain.ConsumeCode{
		Channel:         channel,
		Purpose:         purpose,
		DestinationHash: destinationHash,
		CodeHash:        c.codeHash(destinationHash, purpose, code),
		Now:             now,
	}, onAccepted)
	if err != nil {
		return err
	}
	if !accepted {
		return domain.ErrInvalidCode
	}
	return nil
}

// destinationHash identifies an address without storing it.
func (c *oneTimeCodes) destinationHash(channel domain.CodeChannel, destination string) []byte {
	mac := hmac.New(sha256.New, c.destinationKey)
	mac.Write([]byte(channel))
	mac.Write([]byte{0})
	mac.Write([]byte(destination))
	return mac.Sum(nil)
}

// codeHash binds a code to the address it was sent to and to what it is for,
// so the same digits hash differently for another address or another purpose.
func (c *oneTimeCodes) codeHash(destinationHash []byte, purpose domain.CodePurpose, code string) []byte {
	mac := hmac.New(sha256.New, c.codeKey)
	mac.Write(destinationHash)
	mac.Write([]byte(purpose))
	mac.Write([]byte{0})
	mac.Write([]byte(code))
	return mac.Sum(nil)
}

func (c *oneTimeCodes) findOrCreate(ctx context.Context, channel domain.CodeChannel, destination string) (*domain.User, error) {
	if channel == domain.CodeChannelPhone {
		return c.accounts.FindOrCreateByPhone(ctx, destination)
	}
	return c.accounts.FindOrCreateByEmail(ctx, destination)
}

func (c *oneTimeCodes) claim(ctx context.Context, channel domain.CodeChannel, userID uuid.UUID) error {
	if channel == domain.CodeChannelPhone {
		return c.accounts.ClaimPhone(ctx, userID)
	}
	return c.accounts.ClaimEmail(ctx, userID)
}

// normalizeDestination returns the address in its stored form, or false when
// it is not one codes may be sent to: not valid for the channel, or a phone
// number outside the countries the store texts.
func (c *oneTimeCodes) normalizeDestination(channel domain.CodeChannel, raw string) (string, bool) {
	switch channel {
	case domain.CodeChannelEmail:
		return normalizeEmail(raw)
	case domain.CodeChannelPhone:
		phone, ok := normalizePhone(raw)
		if !ok {
			return "", false
		}
		for _, countryCode := range c.phoneCountryCodes {
			if strings.HasPrefix(phone, "+"+countryCode) {
				return phone, true
			}
		}
		return "", false
	default:
		return "", false
	}
}

// verifiedOn reports whether the account has proved the address it has on the
// channel.
func verifiedOn(user *domain.User, channel domain.CodeChannel) bool {
	if channel == domain.CodeChannelPhone {
		return user.PhoneVerified
	}
	return user.EmailVerified
}

func canSignIn(user *domain.User) bool {
	return user != nil && user.Status == domain.UserStatusActive && validRole(user.Role)
}

// normalizeEmail returns the address in the form it is stored and looked up
// in, or false when it is not a bare address.
func normalizeEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > maxEmailLength {
		return "", false
	}
	// ParseAddress also accepts "Name <address>", which is not something a
	// customer types into an address field.
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return "", false
	}
	return email, true
}

// normalizePhone returns a phone number in E.164 form — "+", a country code and
// the number, at most fifteen digits — or false. Spaces, dashes, dots and
// brackets are dropped and a leading international "00" becomes "+". A number
// without its country code is refused rather than guessed at: the core does not
// know which country a store is in.
func normalizePhone(raw string) (string, bool) {
	var digits strings.Builder
	value := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(value, "+"):
		value = value[1:]
	case strings.HasPrefix(value, "00"):
		value = value[2:]
	default:
		return "", false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == ' ' || r == '-' || r == '.' || r == '(' || r == ')':
		default:
			return "", false
		}
	}
	number := digits.String()
	if len(number) < 8 || len(number) > 15 || number[0] == '0' {
		return "", false
	}
	return "+" + number, true
}

// validCountryCodes checks calling codes: one to three digits, not starting
// with 0.
func validCountryCodes(values []string) ([]string, error) {
	codes := make([]string, 0, len(values))
	for _, value := range values {
		code := strings.TrimPrefix(strings.TrimSpace(value), "+")
		if len(code) < 1 || len(code) > 3 || code[0] == '0' || strings.Trim(code, "0123456789") != "" {
			return nil, fmt.Errorf("phone country code %q is not one to three digits", value)
		}
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		return nil, fmt.Errorf("phone codes require at least one country code to send to")
	}
	return codes, nil
}

func wellFormedCode(code string) bool {
	if len(code) != codeDigits {
		return false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// newCode draws a code uniformly from every six-digit value, leading zeros
// included.
func newCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate one-time code: %w", err)
	}
	return fmt.Sprintf("%0*d", codeDigits, value.Int64()), nil
}

func deriveKey(secret, purpose string) ([]byte, error) {
	key := make([]byte, sha256.Size)
	if _, err := io.ReadFull(hkdf.New(sha256.New, []byte(secret), nil, []byte(purpose)), key); err != nil {
		return nil, fmt.Errorf("derive %s key: %w", purpose, err)
	}
	return key, nil
}
