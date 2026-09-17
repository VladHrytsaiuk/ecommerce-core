package domain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CodeChannel is where a one-time code is sent.
type CodeChannel string

const (
	CodeChannelEmail CodeChannel = "email"
	CodeChannelPhone CodeChannel = "phone"
)

// CodePurpose is what a one-time code is for. A code is accepted only for the
// purpose it was issued for: a sign-in code cannot reset a password.
type CodePurpose string

const (
	// CodePurposeSignIn signs in, registering the address if it is new.
	CodePurposeSignIn CodePurpose = "sign_in"
	// CodePurposeVerifyEmail proves a signed-in account owns its address.
	CodePurposeVerifyEmail CodePurpose = "verify_email"
	// CodePurposeResetPassword sets a new password for the account at the
	// address.
	CodePurposeResetPassword CodePurpose = "reset_password"
)

// ErrInvalidCode covers every code that cannot be used: wrong, expired,
// replaced by a newer one, out of attempts, or sent to an address that is not
// valid. One error for all of them, so the endpoint says nothing about which.
var ErrInvalidCode = errors.New("invalid or expired code")

// ErrInvalidCodeDestination refuses a request for a code to something that is
// not a valid address for its channel.
var ErrInvalidCodeDestination = errors.New("invalid code destination")

// ErrCodesUnavailable refuses a use of one-time codes where the store sends
// none, because nothing is configured to deliver them.
var ErrCodesUnavailable = errors.New("one-time codes are not available")

// ErrCodeDeliveryFailed reports a code that was stored but could not be handed
// to the provider that delivers it. The code still counts towards the limits,
// so a failing provider is not called again at once.
var ErrCodeDeliveryFailed = errors.New("one-time code could not be delivered")

// ErrEmailAlreadyVerified refuses to send a verification code to an address
// that needs none.
var ErrEmailAlreadyVerified = errors.New("email address is already verified")

// ErrNoEmailToVerify refuses to verify an account that has no address.
var ErrNoEmailToVerify = errors.New("account has no email address")

// CodeThrottledError refuses a code because the address has been sent one too
// recently or too often. The limit is per address, not per account, so it
// reveals nothing about whether an account exists.
type CodeThrottledError struct {
	RetryAfter time.Duration
}

func (e *CodeThrottledError) Error() string {
	return fmt.Sprintf("code throttled; retry after %s", e.RetryAfter)
}

type RequestSignInCodeCommand struct {
	Channel     CodeChannel
	Destination string
}

// CodeRequest is what the client may show: when the code stops working and
// when another may be requested.
type CodeRequest struct {
	ExpiresAt   time.Time
	ResendAfter time.Time
}

type VerifySignInCodeCommand struct {
	Channel        CodeChannel
	Destination    string
	Code           string
	GuestSessionID *uuid.UUID
}

type ConfirmEmailCommand struct {
	UserID uuid.UUID
	Code   string
}

type RequestPasswordResetCommand struct {
	Email string
}

type ResetPasswordCommand struct {
	Email          string
	Code           string
	Password       string
	GuestSessionID *uuid.UUID
}

// CodeLimits bounds how many codes one address may be sent, whatever they are
// for. They exist for the person at that address as much as for the store:
// every request is a message delivered to someone who may not have asked for
// it.
type CodeLimits struct {
	Cooldown    time.Duration
	PerHour     int
	PerDay      int
	MaxAttempts int
	// ChannelPerHour caps the codes the whole store sends on the channel in an
	// hour, whoever they are for; zero is no cap. Each text message costs money,
	// and one address at a time is not a limit to someone rotating numbers.
	ChannelPerHour int
}

// IssueCode stores a code. Only HMACs of the address and of the code are passed
// to the store.
type IssueCode struct {
	ID              uuid.UUID
	Channel         CodeChannel
	Purpose         CodePurpose
	DestinationHash []byte
	CodeHash        []byte
	Now             time.Time
	ExpiresAt       time.Time
	Limits          CodeLimits
	// Unthrottled issues the code even when the address is over a limit. It is
	// for a code whose sending cannot be repeated at will — the one sent on
	// registration, which happens once per address. The code still counts
	// towards the limits of the requests that follow.
	Unthrottled bool
}

type ConsumeCode struct {
	Channel         CodeChannel
	Purpose         CodePurpose
	DestinationHash []byte
	CodeHash        []byte
	Now             time.Time
}

// CodeStore persists one-time codes.
type CodeStore interface {
	// Issue refuses with *CodeThrottledError when the address is over a limit.
	// Otherwise it closes the address's earlier codes for the same purpose,
	// stores this one and calls within in the same transaction, with the
	// transaction in its context: a code exists exactly when what within did —
	// queueing its message, creating the account it verifies — did.
	Issue(ctx context.Context, request IssueCode, within func(context.Context) error) error
	// Consume checks the presented code against the address's newest open code
	// for the purpose. A wrong code uses up an attempt and that is committed,
	// which is why it is an outcome (false) rather than an error: an error would
	// roll it back. A right code is closed and onAccepted runs in the same
	// transaction, with the transaction in its context; if onAccepted fails,
	// nothing is committed and the code can be entered again.
	Consume(ctx context.Context, request ConsumeCode, onAccepted func(context.Context) error) (bool, error)
	// PurgeSettled removes codes that can neither be used nor count towards a
	// limit any more.
	PurgeSettled(ctx context.Context, now time.Time, limit int) (int, error)
}

// CodeMessage is a code on its way to the person who asked for it.
type CodeMessage struct {
	// ID is the stored code's id. It makes every message distinct, including
	// two that happen to carry the same six digits.
	ID          uuid.UUID
	Purpose     CodePurpose
	Destination string
	Code        string
	// Lifetime is how long the code works, for the message to say so.
	Lifetime time.Duration
}

// CodeSender delivers codes on one channel.
type CodeSender interface {
	Channel() CodeChannel
	// Transactional reports how SendCode delivers. True: it queues the message
	// in the transaction that stores the code, so it must perform no network
	// I/O. False: it delivers directly and is called after that transaction
	// commits, never holding it open across a provider call.
	Transactional() bool
	SendCode(ctx context.Context, message CodeMessage) error
}

// CodeAccounts reads and changes the accounts a code proves an address for. It
// joins the transaction carried in ctx.
type CodeAccounts interface {
	// FindByEmail returns the account with this address, or ErrUserNotFound.
	FindByEmail(ctx context.Context, email string) (*User, error)
	// FindOrCreateByEmail returns the account with this address, creating an
	// active customer account with the address already verified when there is
	// none.
	FindOrCreateByEmail(ctx context.Context, email string) (*User, error)
	// ClaimEmail marks the account's address verified and removes its
	// password. See AuthService.VerifySignInCode for why the password goes.
	ClaimEmail(ctx context.Context, userID uuid.UUID) error
	// FindOrCreateByPhone is FindOrCreateByEmail for a phone number in E.164
	// form: an account created here has the number verified.
	FindOrCreateByPhone(ctx context.Context, phone string) (*User, error)
	// ClaimPhone is ClaimEmail for the account's phone number.
	ClaimPhone(ctx context.Context, userID uuid.UUID) error
	// VerifyEmail marks the account's address verified if it is still email.
	// It reports false when the account's address has changed since.
	VerifyEmail(ctx context.Context, userID uuid.UUID, email string) (bool, error)
	// SetPassword replaces the account's password and marks its address
	// verified: the code that allowed it was read at that address.
	SetPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
}
