package application

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/google/uuid"
)

var (
	countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)
	genderPattern      = regexp.MustCompile(`^[a-z][a-z_-]{0,31}$`)
	metadataKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

// CustomerProfileService owns typed self-service profile and address-book
// mutations. Its caller must derive userID from authentication, never JSON.
type CustomerProfileService struct {
	repository identityDomain.CustomerProfileRepository
}

func NewCustomerProfileService(repository identityDomain.CustomerProfileRepository) *CustomerProfileService {
	return &CustomerProfileService{repository: repository}
}

func (s *CustomerProfileService) GetCustomerProfile(ctx context.Context, userID uuid.UUID) (*identityDomain.CustomerProfile, error) {
	if s.repository == nil || userID == uuid.Nil {
		return nil, identityDomain.ErrInvalidCustomerProfile
	}
	return s.repository.GetCustomerProfile(ctx, userID)
}

func (s *CustomerProfileService) PatchCustomerProfile(ctx context.Context, userID uuid.UUID, patch identityDomain.CustomerProfilePatch) (*identityDomain.CustomerProfile, error) {
	if s.repository == nil || userID == uuid.Nil {
		return nil, identityDomain.ErrInvalidCustomerProfile
	}
	current, err := s.repository.GetCustomerProfile(ctx, userID)
	if err != nil && !errors.Is(err, identityDomain.ErrProfileNotFound) {
		return nil, err
	}
	if current == nil {
		current = &identityDomain.CustomerProfile{UserID: userID, Metadata: map[string]json.RawMessage{}}
	}
	if patch.DateOfBirth != nil {
		date := normalizeDate(*patch.DateOfBirth)
		if date.IsZero() || date.After(time.Now().UTC()) {
			return nil, identityDomain.ErrInvalidCustomerProfile
		}
		current.DateOfBirth = &date
	}
	if patch.Gender != nil {
		gender := strings.ToLower(strings.TrimSpace(*patch.Gender))
		if gender != "" && !genderPattern.MatchString(gender) {
			return nil, identityDomain.ErrInvalidCustomerProfile
		}
		current.Gender = gender
	}
	if patch.Metadata != nil {
		if err := validateMetadata(patch.Metadata); err != nil {
			return nil, err
		}
		current.Metadata = cloneMetadata(patch.Metadata)
	}
	return s.repository.UpsertCustomerProfile(ctx, *current)
}

func (s *CustomerProfileService) ListCustomerAddresses(ctx context.Context, userID uuid.UUID) ([]identityDomain.CustomerAddress, error) {
	if s.repository == nil || userID == uuid.Nil {
		return nil, identityDomain.ErrInvalidCustomerAddress
	}
	return s.repository.ListCustomerAddresses(ctx, userID)
}

func (s *CustomerProfileService) CreateCustomerAddress(ctx context.Context, userID uuid.UUID, input identityDomain.AddressInput) (*identityDomain.CustomerAddress, error) {
	address, err := validAddress(userID, uuid.New(), input)
	if err != nil {
		return nil, err
	}
	if s.repository == nil {
		return nil, identityDomain.ErrInvalidCustomerAddress
	}
	return s.repository.CreateCustomerAddress(ctx, address)
}

func (s *CustomerProfileService) UpdateCustomerAddress(ctx context.Context, userID, addressID uuid.UUID, input identityDomain.AddressInput) (*identityDomain.CustomerAddress, error) {
	address, err := validAddress(userID, addressID, input)
	if err != nil {
		return nil, err
	}
	if s.repository == nil {
		return nil, identityDomain.ErrInvalidCustomerAddress
	}
	return s.repository.UpdateCustomerAddress(ctx, address)
}

func (s *CustomerProfileService) DeleteCustomerAddress(ctx context.Context, userID, addressID uuid.UUID) error {
	if s.repository == nil || userID == uuid.Nil || addressID == uuid.Nil {
		return identityDomain.ErrInvalidCustomerAddress
	}
	return s.repository.DeleteCustomerAddress(ctx, userID, addressID)
}

// GetAvailableProfileFields is the port used by Checkout. Missing profile is
// a valid incomplete state, not an infrastructure error.
func (s *CustomerProfileService) GetAvailableProfileFields(ctx context.Context, userID uuid.UUID) (map[string]bool, error) {
	profile, err := s.GetCustomerProfile(ctx, userID)
	if errors.Is(err, identityDomain.ErrProfileNotFound) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	// Repositories must normally return ErrProfileNotFound for an absent row,
	// but keep the boundary total even if an alternate adapter returns (nil,
	// nil). Checkout will then report the configured fields as incomplete.
	if profile == nil {
		return map[string]bool{}, nil
	}
	fields := map[string]bool{}
	if profile.DateOfBirth != nil {
		fields["date_of_birth"] = true
	}
	if strings.TrimSpace(profile.Gender) != "" {
		fields["gender"] = true
	}
	for key, value := range profile.Metadata {
		if len(value) > 0 && string(value) != "null" {
			fields[key] = true
		}
	}
	return fields, nil
}

func validAddress(userID, addressID uuid.UUID, input identityDomain.AddressInput) (identityDomain.CustomerAddress, error) {
	address := identityDomain.CustomerAddress{ID: addressID, UserID: userID, Title: strings.TrimSpace(input.Title), Country: strings.ToUpper(strings.TrimSpace(input.Country)), City: strings.TrimSpace(input.City), Line1: strings.TrimSpace(input.Line1), Line2: strings.TrimSpace(input.Line2), ZipCode: strings.TrimSpace(input.ZipCode), IsDefault: input.IsDefault}
	if userID == uuid.Nil || addressID == uuid.Nil || address.Title == "" || address.City == "" || address.Line1 == "" || address.ZipCode == "" || !countryCodePattern.MatchString(address.Country) || utf8.RuneCountInString(address.Title) > 100 || utf8.RuneCountInString(address.City) > 120 || utf8.RuneCountInString(address.Line1) > 255 || utf8.RuneCountInString(address.Line2) > 255 || utf8.RuneCountInString(address.ZipCode) > 32 {
		return identityDomain.CustomerAddress{}, identityDomain.ErrInvalidCustomerAddress
	}
	return address, nil
}

func validateMetadata(metadata map[string]json.RawMessage) error {
	if len(metadata) > 32 {
		return identityDomain.ErrInvalidCustomerProfile
	}
	for key, value := range metadata {
		if !metadataKeyPattern.MatchString(key) || len(value) == 0 || len(value) > 4096 || string(value) == "null" || !json.Valid(value) {
			return identityDomain.ErrInvalidCustomerProfile
		}
	}
	return nil
}

func cloneMetadata(value map[string]json.RawMessage) map[string]json.RawMessage {
	cloned := make(map[string]json.RawMessage, len(value))
	for key, raw := range value {
		cloned[key] = append(json.RawMessage(nil), raw...)
	}
	return cloned
}

func normalizeDate(value time.Time) time.Time {
	return time.Date(value.UTC().Year(), value.UTC().Month(), value.UTC().Day(), 0, 0, 0, 0, time.UTC)
}

var _ identityDomain.CustomerProfileService = (*CustomerProfileService)(nil)
