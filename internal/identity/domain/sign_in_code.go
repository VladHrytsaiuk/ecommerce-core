package domain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SignInCodeChannel is where a one-time sign-in code is sent.
type SignInCodeChannel string

const SignInCodeEmail SignInCodeChannel = "email"

// ErrInvalidSignInCode covers every code that cannot be exchanged for a
// session: wrong, expired, replaced by a newer one, out of attempts, or sent to
// an address that is not valid. One error for all of them, so the endpoint
// says nothing about which.
var ErrInvalidSignInCode = errors.New("invalid or expired sign-in code")

// ErrInvalidSignInDestination refuses a request for a code to something that
// is not a valid address for its channel.
var ErrInvalidSignInDestination = errors.New("invalid sign-in code destination")

// SignInCodeThrottledError refuses a code because the address has been sent
// one too recently or too often. The limit is per address, not per account, so
// it reveals nothing about whether an account exists.
type SignInCodeThrottledError struct {
	RetryAfter time.Duration
}

func (e *SignInCodeThrottledError) Error() string {
	return fmt.Sprintf("sign-in code throttled; retry after %s", e.RetryAfter)
}

type RequestSignInCodeCommand struct {
	Channel     SignInCodeChannel
	Destination string
}

// SignInCodeRequest is what the client may show: when the code stops working
// and when another may be requested.
type SignInCodeRequest struct {
	ExpiresAt   time.Time
	ResendAfter time.Time
}

type VerifySignInCodeCommand struct {
	Channel        SignInCodeChannel
	Destination    string
	Code           string
	GuestSessionID *uuid.UUID
}

// SignInCodeLimits bounds how many codes one address may be sent. They exist
// for the person at that address as much as for the store: every request is a
// message delivered to someone who may not have asked for it.
type SignInCodeLimits struct {
	Cooldown    time.Duration
	PerHour     int
	PerDay      int
	MaxAttempts int
}

// IssueSignInCode stores a code. Only HMACs of the address and of the code are
// passed to the store.
type IssueSignInCode struct {
	ID              uuid.UUID
	Channel         SignInCodeChannel
	DestinationHash []byte
	CodeHash        []byte
	Now             time.Time
	ExpiresAt       time.Time
	Limits          SignInCodeLimits
}

type ConsumeSignInCode struct {
	Channel         SignInCodeChannel
	DestinationHash []byte
	CodeHash        []byte
	Now             time.Time
}

// SignInCodeStore persists sign-in codes.
type SignInCodeStore interface {
	// Issue refuses with *SignInCodeThrottledError when the address is over a
	// limit. Otherwise it closes the address's earlier codes, stores this one
	// and calls deliver in the same transaction, with the transaction in its
	// context: a code exists exactly when its message was queued.
	Issue(ctx context.Context, request IssueSignInCode, deliver func(context.Context) error) error
	// Consume checks the presented code against the address's newest open code.
	// A wrong code uses up an attempt and that is committed, which is why it is
	// an outcome (false) rather than an error: an error would roll it back.
	// A right code is closed and onAccepted runs in the same transaction, with
	// the transaction in its context; if onAccepted fails, nothing is committed
	// and the code can be entered again.
	Consume(ctx context.Context, request ConsumeSignInCode, onAccepted func(context.Context) error) (bool, error)
	// PurgeSettled removes codes that can neither be used nor count towards a
	// limit any more.
	PurgeSettled(ctx context.Context, now time.Time, limit int) (int, error)
}

// SignInCodeMessage is a code on its way to the person who asked for it.
type SignInCodeMessage struct {
	// ID is the stored code's id. It makes every message distinct, including
	// two that happen to carry the same six digits.
	ID          uuid.UUID
	Destination string
	Code        string
	// Lifetime is how long the code works, for the message to say so.
	Lifetime time.Duration
}

// SignInCodeSender delivers codes on one channel. It is called inside the
// transaction that stores the code, so it must queue the message rather than
// perform network I/O.
type SignInCodeSender interface {
	Channel() SignInCodeChannel
	SendSignInCode(ctx context.Context, message SignInCodeMessage) error
}

// CodeSignInAccounts resolves the account a verified address signs in to. Its
// methods run inside the transaction that accepted the code.
type CodeSignInAccounts interface {
	// FindOrCreateByEmail returns the account with this address, creating an
	// active customer account with the address already verified when there is
	// none.
	FindOrCreateByEmail(ctx context.Context, email string) (*User, error)
	// ClaimEmail marks the account's address verified and removes its
	// password. See AuthService.VerifySignInCode for why the password goes.
	ClaimEmail(ctx context.Context, userID uuid.UUID) error
}
