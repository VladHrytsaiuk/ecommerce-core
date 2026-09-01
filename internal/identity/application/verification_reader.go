// Package application contains narrow cross-module Identity adapters.
package application

import (
	"context"

	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/google/uuid"
)

// VerificationReader adapts Identity's user repository to Checkout's small
// verification read port. No contact values, password hashes or tokens cross
// the module boundary.
type VerificationReader struct {
	statuses identityDomain.VerificationStatusReader
}

func NewVerificationReader(statuses identityDomain.VerificationStatusReader) *VerificationReader {
	return &VerificationReader{statuses: statuses}
}

func (r *VerificationReader) GetVerificationStatus(ctx context.Context, customerID uuid.UUID) (checkoutDomain.VerificationStatus, error) {
	if r == nil || r.statuses == nil {
		// A miswired optional module must fail checkout closed, not panic or
		// silently accept an unverified customer.
		return checkoutDomain.VerificationStatus{}, checkoutDomain.ErrVerificationReaderUnavailable
	}
	status, err := r.statuses.GetVerificationStatus(ctx, customerID)
	if err != nil {
		return checkoutDomain.VerificationStatus{}, err
	}
	return checkoutDomain.VerificationStatus{IsEmailVerified: status.EmailVerified, IsPhoneVerified: status.PhoneVerified}, nil
}

// CheckoutProfileReader adapts the Identity-owned service to Checkout's
// presence-only profile port. No date, gender, address or metadata value
// crosses the module boundary.
type CheckoutProfileReader struct {
	profiles identityDomain.CustomerProfileService
}

func NewCheckoutProfileReader(profiles identityDomain.CustomerProfileService) *CheckoutProfileReader {
	return &CheckoutProfileReader{profiles: profiles}
}

func (r *CheckoutProfileReader) GetAvailableProfileFields(ctx context.Context, customerID uuid.UUID) (map[string]bool, error) {
	if r == nil || r.profiles == nil {
		// Preserve the policy invariant if composition is incomplete: Checkout
		// reports a controlled configuration error instead of dereferencing nil.
		return nil, checkoutDomain.ErrProfileReaderUnavailable
	}
	return r.profiles.GetAvailableProfileFields(ctx, customerID)
}
