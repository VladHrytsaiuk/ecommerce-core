package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

type ProfileService struct {
	policy domain.ProfilePolicy
	repo   domain.ProfileRepository
}

func NewProfileService(policy domain.ProfilePolicy, repo domain.ProfileRepository) *ProfileService {
	return &ProfileService{policy: policy, repo: repo}
}

func (s *ProfileService) GetProfile(ctx context.Context, userID uuid.UUID) (*domain.Profile, error) {
	if s.repo == nil || userID == uuid.Nil {
		return nil, domain.ErrInvalidProfile
	}
	return s.repo.FindByUserID(ctx, userID)
}

// UpdateProfile applies a customer patch to the profile. Server-managed fields
// are preserved even though persistence stores the full JSON document.
func (s *ProfileService) UpdateProfile(ctx context.Context, userID uuid.UUID, raw []byte) (*domain.Profile, error) {
	if s.repo == nil || userID == uuid.Nil || s.policy.SchemaVersion <= 0 {
		return nil, domain.ErrInvalidProfile
	}
	patch, err := parseAttributes(raw)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]domain.ProfileFieldConfig, len(s.policy.Fields))
	for _, field := range s.policy.Fields {
		if field.Key == "" || fields[field.Key].Key != "" {
			return nil, domain.ErrInvalidProfile
		}
		fields[field.Key] = field
	}
	current, err := s.repo.FindByUserID(ctx, userID)
	if err != nil && !errors.Is(err, domain.ErrProfileNotFound) {
		return nil, err
	}
	attributes := make(map[string]json.RawMessage)
	if current != nil {
		for key, value := range current.Attributes {
			attributes[key] = append(json.RawMessage(nil), value...)
		}
	}
	for key, value := range patch {
		field, ok := fields[key]
		if !ok || !field.CustomerWritable || isJSONNull(value) {
			return nil, domain.ErrInvalidProfile
		}
		attributes[key] = append(json.RawMessage(nil), value...)
	}
	for key, value := range attributes {
		field, ok := fields[key]
		if !ok || isJSONNull(value) {
			return nil, domain.ErrInvalidProfile
		}
		if err := validateField(field, value); err != nil {
			return nil, err
		}
	}
	for _, field := range s.policy.Fields {
		if field.Required && isJSONNull(attributes[field.Key]) {
			return nil, domain.ErrInvalidProfile
		}
	}
	revision := 0
	if current != nil {
		revision = current.Revision
	}
	return s.repo.Upsert(ctx, domain.Profile{UserID: userID, Attributes: attributes, SchemaVersion: s.policy.SchemaVersion, Revision: revision})
}

func parseAttributes(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var attributes map[string]json.RawMessage
	if err := decoder.Decode(&attributes); err != nil || attributes == nil {
		return nil, domain.ErrInvalidProfile
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, domain.ErrInvalidProfile
	}
	return attributes, nil
}

func isJSONNull(value json.RawMessage) bool {
	return len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func validateField(field domain.ProfileFieldConfig, raw json.RawMessage) error {
	switch field.Type {
	case domain.ProfileFieldString:
		var value string
		if json.Unmarshal(raw, &value) != nil || field.MaxLength > 0 && utf8.RuneCountInString(value) > field.MaxLength {
			return domain.ErrInvalidProfile
		}
	case domain.ProfileFieldEnum:
		var value string
		if json.Unmarshal(raw, &value) != nil || !contains(field.AllowedValues, value) {
			return domain.ErrInvalidProfile
		}
	case domain.ProfileFieldBool:
		var value bool
		if json.Unmarshal(raw, &value) != nil || (string(bytes.TrimSpace(raw)) != "true" && string(bytes.TrimSpace(raw)) != "false") {
			return domain.ErrInvalidProfile
		}
	case domain.ProfileFieldDate:
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return domain.ErrInvalidProfile
		}
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil || parsed.Format("2006-01-02") != value {
			return domain.ErrInvalidProfile
		}
	case domain.ProfileFieldNumber:
		value := strings.TrimSpace(string(raw))
		if _, ok := new(big.Rat).SetString(value); !ok || strings.ContainsAny(value, "eE") {
			return domain.ErrInvalidProfile
		}
	default:
		return domain.ErrInvalidProfile
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

var _ domain.ProfileService = (*ProfileService)(nil)
