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
	signInCodeDigits = 6
	// maxEmailLength is users.email's column width.
	maxEmailLength = 320

	signInCodeDestinationKeyPurpose = "identity:sign-in-code:destination:v1"
	signInCodeKeyPurpose            = "identity:sign-in-code:code:v1"
)

// signInCodeLimits allow a person a resend a minute and a handful an hour.
// Five attempts per code, at most ten codes a day, leaves a guesser fifty
// tries a day at a million combinations — and fifty messages in the inbox of
// the person being targeted.
var signInCodeLimits = domain.SignInCodeLimits{
	Cooldown:    time.Minute,
	PerHour:     5,
	PerDay:      10,
	MaxAttempts: 5,
}

// MinSignInCodeTTL and MaxSignInCodeTTL bound how long a code works: long
// enough to switch to the mailbox and back, short enough that a code read over
// someone's shoulder is soon useless.
const (
	MinSignInCodeTTL = time.Minute
	MaxSignInCodeTTL = 30 * time.Minute
)

type signInCodes struct {
	store          domain.SignInCodeStore
	accounts       domain.CodeSignInAccounts
	senders        map[domain.SignInCodeChannel]domain.SignInCodeSender
	destinationKey []byte
	codeKey        []byte
	ttl            time.Duration
}

// WithSignInCodes enables signing in with a one-time code, on the channels of
// the senders given. secret is the server secret the code hashing keys are
// derived from; it is never used directly.
func (s *AuthService) WithSignInCodes(store domain.SignInCodeStore, accounts domain.CodeSignInAccounts, secret string, ttl time.Duration, senders ...domain.SignInCodeSender) (*AuthService, error) {
	if store == nil || accounts == nil || len(senders) == 0 {
		return nil, fmt.Errorf("sign-in codes require a store, an account resolver and at least one sender")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("sign-in codes require a server secret")
	}
	if ttl < MinSignInCodeTTL || ttl > MaxSignInCodeTTL {
		return nil, fmt.Errorf("sign-in code lifetime must be between %s and %s", MinSignInCodeTTL, MaxSignInCodeTTL)
	}
	codes := &signInCodes{store: store, accounts: accounts, ttl: ttl, senders: make(map[domain.SignInCodeChannel]domain.SignInCodeSender, len(senders))}
	for _, sender := range senders {
		if sender == nil {
			return nil, fmt.Errorf("sign-in code sender is nil")
		}
		if sender.Channel() != domain.SignInCodeEmail {
			return nil, fmt.Errorf("sign-in code channel %q is not supported", sender.Channel())
		}
		if _, duplicate := codes.senders[sender.Channel()]; duplicate {
			return nil, fmt.Errorf("sign-in code channel %q has more than one sender", sender.Channel())
		}
		codes.senders[sender.Channel()] = sender
	}
	var err error
	if codes.destinationKey, err = deriveKey(secret, signInCodeDestinationKeyPurpose); err != nil {
		return nil, err
	}
	if codes.codeKey, err = deriveKey(secret, signInCodeKeyPurpose); err != nil {
		return nil, err
	}
	s.signInCodes = codes
	return s, nil
}

// RequestSignInCode sends a code to an address, whether or not it has an
// account: asking for a code is how an account is registered, and answering
// differently for a known address would say which addresses are known.
func (s *AuthService) RequestSignInCode(ctx context.Context, command domain.RequestSignInCodeCommand) (domain.SignInCodeRequest, error) {
	sender, err := s.signInCodeSender(command.Channel)
	if err != nil {
		return domain.SignInCodeRequest{}, err
	}
	destination, ok := normalizeDestination(command.Channel, command.Destination)
	if !ok {
		return domain.SignInCodeRequest{}, domain.ErrInvalidSignInDestination
	}
	code, err := newSignInCode()
	if err != nil {
		return domain.SignInCodeRequest{}, err
	}
	codes := s.signInCodes
	destinationHash := codes.destinationHash(command.Channel, destination)
	now := s.now().UTC()
	expiresAt := now.Add(codes.ttl)
	message := domain.SignInCodeMessage{ID: uuid.New(), Destination: destination, Code: code, Lifetime: codes.ttl}
	err = codes.store.Issue(ctx, domain.IssueSignInCode{
		ID:              message.ID,
		Channel:         command.Channel,
		DestinationHash: destinationHash,
		CodeHash:        codes.codeHash(destinationHash, code),
		Now:             now,
		ExpiresAt:       expiresAt,
		Limits:          signInCodeLimits,
	}, func(txCtx context.Context) error {
		return sender.SendSignInCode(txCtx, message)
	})
	if err != nil {
		return domain.SignInCodeRequest{}, err
	}
	return domain.SignInCodeRequest{ExpiresAt: expiresAt, ResendAfter: now.Add(signInCodeLimits.Cooldown)}, nil
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
	if _, err := s.signInCodeSender(command.Channel); err != nil {
		return domain.Session{}, err
	}
	destination, ok := normalizeDestination(command.Channel, command.Destination)
	code := strings.TrimSpace(command.Code)
	if !ok || !wellFormedSignInCode(code) {
		return domain.Session{}, domain.ErrInvalidSignInCode
	}
	codes := s.signInCodes
	destinationHash := codes.destinationHash(command.Channel, destination)
	now := s.now().UTC()
	var user *domain.User
	accepted, err := codes.store.Consume(ctx, domain.ConsumeSignInCode{
		Channel:         command.Channel,
		DestinationHash: destinationHash,
		CodeHash:        codes.codeHash(destinationHash, code),
		Now:             now,
	}, func(txCtx context.Context) error {
		found, err := codes.accounts.FindOrCreateByEmail(txCtx, destination)
		if err != nil {
			return err
		}
		if found.Status != domain.UserStatusActive || !validRole(found.Role) {
			// Refused inside the transaction, so the code is not spent on an
			// account that cannot sign in.
			return domain.ErrInvalidCredentials
		}
		if !found.EmailVerified {
			if err := codes.accounts.ClaimEmail(txCtx, found.ID); err != nil {
				return err
			}
			if s.refreshTokens != nil {
				if err := s.refreshTokens.RevokeUser(txCtx, found.ID, now); err != nil {
					return err
				}
			}
			found.EmailVerified, found.PasswordHash = true, ""
		}
		user = found
		return nil
	})
	if err != nil {
		return domain.Session{}, err
	}
	if !accepted {
		return domain.Session{}, domain.ErrInvalidSignInCode
	}
	return s.finishLogin(ctx, user, command.GuestSessionID)
}

func (s *AuthService) signInCodeSender(channel domain.SignInCodeChannel) (domain.SignInCodeSender, error) {
	if s.signInCodes == nil || s.tokens == nil || s.accessTTL <= 0 {
		return nil, domain.ErrSignInMethodDisabled
	}
	sender, ok := s.signInCodes.senders[channel]
	if !ok {
		return nil, domain.ErrSignInMethodDisabled
	}
	return sender, nil
}

// destinationHash identifies an address without storing it.
func (c *signInCodes) destinationHash(channel domain.SignInCodeChannel, destination string) []byte {
	mac := hmac.New(sha256.New, c.destinationKey)
	mac.Write([]byte(channel))
	mac.Write([]byte{0})
	mac.Write([]byte(destination))
	return mac.Sum(nil)
}

// codeHash binds a code to the address it was sent to, so the same digits sent
// to two addresses hash differently.
func (c *signInCodes) codeHash(destinationHash []byte, code string) []byte {
	mac := hmac.New(sha256.New, c.codeKey)
	mac.Write(destinationHash)
	mac.Write([]byte(code))
	return mac.Sum(nil)
}

func normalizeDestination(channel domain.SignInCodeChannel, raw string) (string, bool) {
	switch channel {
	case domain.SignInCodeEmail:
		email := strings.ToLower(strings.TrimSpace(raw))
		if email == "" || len(email) > maxEmailLength {
			return "", false
		}
		// A bare address only: ParseAddress also accepts "Name <address>", which
		// is not something a customer types into a sign-in field.
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email {
			return "", false
		}
		return email, true
	default:
		return "", false
	}
}

func wellFormedSignInCode(code string) bool {
	if len(code) != signInCodeDigits {
		return false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// newSignInCode draws a code uniformly from every six-digit value, leading
// zeros included.
func newSignInCode() (string, error) {
	limit := big.NewInt(1_000_000)
	value, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", fmt.Errorf("generate sign-in code: %w", err)
	}
	return fmt.Sprintf("%0*d", signInCodeDigits, value.Int64()), nil
}

func deriveKey(secret, purpose string) ([]byte, error) {
	key := make([]byte, sha256.Size)
	if _, err := io.ReadFull(hkdf.New(sha256.New, []byte(secret), nil, []byte(purpose)), key); err != nil {
		return nil, fmt.Errorf("derive %s key: %w", purpose, err)
	}
	return key, nil
}
