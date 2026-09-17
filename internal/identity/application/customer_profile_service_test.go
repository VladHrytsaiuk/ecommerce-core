package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/google/uuid"
)

func TestCustomerProfileAvailableFieldsTreatsMissingProfileAsIncomplete(t *testing.T) {
	service := NewCustomerProfileService(&customerProfilesFake{getErr: identityDomain.ErrProfileNotFound})
	fields, err := service.GetAvailableProfileFields(context.Background(), uuid.New())
	if err != nil || len(fields) != 0 {
		t.Fatalf("GetAvailableProfileFields() = (%+v, %v)", fields, err)
	}
}

func TestCustomerProfileAvailableFieldsTreatsNilProfileAsIncomplete(t *testing.T) {
	service := NewCustomerProfileService(&customerProfilesFake{})
	fields, err := service.GetAvailableProfileFields(context.Background(), uuid.New())
	if err != nil || len(fields) != 0 {
		t.Fatalf("GetAvailableProfileFields() = (%+v, %v)", fields, err)
	}
}

func TestCustomerProfilePatchAndAddressAreCustomerScoped(t *testing.T) {
	userID := uuid.New()
	repository := &customerProfilesFake{profile: &identityDomain.CustomerProfile{UserID: userID, Metadata: map[string]json.RawMessage{}}}
	service := NewCustomerProfileService(repository)
	birthDate := time.Date(1990, 4, 3, 0, 0, 0, 0, time.UTC)
	gender := "female"
	profile, err := service.PatchCustomerProfile(context.Background(), userID, identityDomain.CustomerProfilePatch{DateOfBirth: &birthDate, Gender: &gender, Metadata: map[string]json.RawMessage{"loyalty_tier": json.RawMessage(`"silver"`)}})
	if err != nil || profile.DateOfBirth == nil || profile.Gender != "female" {
		t.Fatalf("PatchCustomerProfile() = (%+v, %v)", profile, err)
	}
	address, err := service.CreateCustomerAddress(context.Background(), userID, identityDomain.AddressInput{Title: "Home", Country: "es", City: "Madrid", Line1: "Calle Mayor 1", ZipCode: "28013", IsDefault: true})
	if err != nil || address.UserID != userID || address.Country != "ES" {
		t.Fatalf("CreateCustomerAddress() = (%+v, %v)", address, err)
	}
}

type customerProfilesFake struct {
	profile *identityDomain.CustomerProfile
	getErr  error
	address identityDomain.CustomerAddress
}

func (f *customerProfilesFake) GetCustomerProfile(context.Context, uuid.UUID) (*identityDomain.CustomerProfile, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.profile, nil
}
func (f *customerProfilesFake) UpsertCustomerProfile(_ context.Context, profile identityDomain.CustomerProfile) (*identityDomain.CustomerProfile, error) {
	f.profile = &profile
	return &profile, nil
}
func (f *customerProfilesFake) ListCustomerAddresses(context.Context, uuid.UUID) ([]identityDomain.CustomerAddress, error) {
	return nil, nil
}
func (f *customerProfilesFake) CreateCustomerAddress(_ context.Context, address identityDomain.CustomerAddress) (*identityDomain.CustomerAddress, error) {
	f.address = address
	return &address, nil
}
func (f *customerProfilesFake) UpdateCustomerAddress(context.Context, identityDomain.CustomerAddress) (*identityDomain.CustomerAddress, error) {
	return nil, errors.New("unexpected update")
}
func (f *customerProfilesFake) DeleteCustomerAddress(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("unexpected delete")
}
