package application

import (
	"context"
	"errors"
	"testing"

	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/google/uuid"
)

func TestVerificationReaderReturnsOnlyVerificationProjection(t *testing.T) {
	userID := uuid.New()
	reader := NewVerificationReader(verificationStatusesFake{status: identityDomain.UserVerificationStatus{EmailVerified: true, PhoneVerified: false}})

	status, err := reader.GetVerificationStatus(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetVerificationStatus() error = %v", err)
	}
	if !status.IsEmailVerified || status.IsPhoneVerified {
		t.Fatalf("status = %+v", status)
	}
}

func TestVerificationReaderPropagatesRepositoryError(t *testing.T) {
	want := errors.New("identity unavailable")
	reader := NewVerificationReader(verificationStatusesFake{err: want})
	if _, err := reader.GetVerificationStatus(context.Background(), uuid.New()); !errors.Is(err, want) {
		t.Fatalf("GetVerificationStatus() error = %v, want %v", err, want)
	}
}

func TestVerificationReaderFailsClosedWhenNotWired(t *testing.T) {
	reader := NewVerificationReader(nil)
	if _, err := reader.GetVerificationStatus(context.Background(), uuid.New()); !errors.Is(err, checkoutDomain.ErrVerificationReaderUnavailable) {
		t.Fatalf("GetVerificationStatus() error = %v, want verification reader unavailable", err)
	}
}

func TestCheckoutProfileReaderFailsClosedWhenNotWired(t *testing.T) {
	reader := NewCheckoutProfileReader(nil)
	if _, err := reader.GetAvailableProfileFields(context.Background(), uuid.New()); !errors.Is(err, checkoutDomain.ErrProfileReaderUnavailable) {
		t.Fatalf("GetAvailableProfileFields() error = %v, want profile reader unavailable", err)
	}
}

type verificationStatusesFake struct {
	status identityDomain.UserVerificationStatus
	err    error
}

func (f verificationStatusesFake) GetVerificationStatus(context.Context, uuid.UUID) (identityDomain.UserVerificationStatus, error) {
	return f.status, f.err
}
