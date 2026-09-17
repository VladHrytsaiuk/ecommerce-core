// Package sms delivers Identity's one-time codes as text messages.
package sms

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	identity "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

// TextSender is an SMS provider. phone is in E.164 form.
type TextSender interface {
	Send(ctx context.Context, phone, text string) error
}

const (
	codePlaceholder    = "{code}"
	minutesPlaceholder = "{minutes}"
	// maxMessageRunes keeps a code to a few SMS segments at most; a template
	// longer than this is a mistake that would be paid for on every text.
	maxMessageRunes = 300
)

// CodeTexter sends a code in a text built from a template.
type CodeTexter struct {
	sender  TextSender
	message string
}

// NewCodeTexter checks the template: it must contain {code}, may contain
// {minutes}, and must be short enough to be a text.
func NewCodeTexter(sender TextSender, message string) (*CodeTexter, error) {
	if sender == nil {
		return nil, fmt.Errorf("SMS code texter requires a sender")
	}
	message = strings.TrimSpace(message)
	if strings.Count(message, codePlaceholder) != 1 {
		return nil, fmt.Errorf("SMS code message must contain %s exactly once", codePlaceholder)
	}
	if utf8.RuneCountInString(message) > maxMessageRunes {
		return nil, fmt.Errorf("SMS code message is longer than %d characters", maxMessageRunes)
	}
	return &CodeTexter{sender: sender, message: message}, nil
}

func (*CodeTexter) Channel() identity.CodeChannel { return identity.CodeChannelPhone }

// Transactional is false: the provider is called over the network, after the
// code is stored.
func (*CodeTexter) Transactional() bool { return false }

func (t *CodeTexter) SendCode(ctx context.Context, message identity.CodeMessage) error {
	if t == nil || t.sender == nil || message.Purpose != identity.CodePurposeSignIn || strings.TrimSpace(message.Destination) == "" || message.Code == "" || message.Lifetime <= 0 {
		return fmt.Errorf("invalid SMS code message")
	}
	text := strings.NewReplacer(
		codePlaceholder, message.Code,
		minutesPlaceholder, strconv.Itoa(int(math.Ceil(message.Lifetime.Minutes()))),
	).Replace(t.message)
	return t.sender.Send(ctx, message.Destination, text)
}

var _ identity.CodeSender = (*CodeTexter)(nil)
